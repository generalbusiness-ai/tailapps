// Package engine coordinates definition lifecycle, ingestion obligations,
// flat projection waves and aligned query snapshots for one resident process.
package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing/fstest"
	"time"

	"golang.org/x/sys/unix"

	"github.com/generalbusiness-ai/tailapp/internal/definition"
	"github.com/generalbusiness-ai/tailapp/internal/inbox"
	"github.com/generalbusiness-ai/tailapp/internal/ingest"
	"github.com/generalbusiness-ai/tailapp/internal/profile"
	"github.com/generalbusiness-ai/tailapp/internal/projection"
	"github.com/generalbusiness-ai/tailapp/internal/query"
	"github.com/generalbusiness-ai/tailapp/tailapps"
)

type Engine struct {
	home           string
	queue          *inbox.Queue
	registry       *definition.Registry
	active         map[string]*projection.Projection
	mu             sync.Mutex
	notify         chan struct{}
	stop           chan struct{}
	done           chan struct{}
	lockFile       *os.File
	upgradePending map[string]bool
}

type Status struct {
	Profile        string                         `json:"profile"`
	IngestionReady bool                           `json:"ingestion_ready"`
	Inbox          inbox.Stats                    `json:"inbox"`
	Apps           map[string]projection.Frontier `json:"apps"`
}

func InitHome(home string) error {
	if home == "" {
		return errors.New("TAILAPP_HOME is required")
	}
	if err := os.MkdirAll(filepath.Join(home, "projections"), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(home, "tmp"), 0o700); err != nil {
		return err
	}
	return os.Chmod(home, 0o700)
}

func Open(ctx context.Context, home string) (*Engine, error) {
	if err := InitHome(home); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(home, "engine.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lockFile.Close()
		return nil, errors.New("engine_already_running")
	}
	control := filepath.Join(home, "control.sqlite")
	queue, err := inbox.Open(control, inbox.Limits{})
	if err != nil {
		lockFile.Close()
		return nil, err
	}
	registry, err := definition.Open(control)
	if err != nil {
		queue.Close()
		lockFile.Close()
		return nil, err
	}
	engine := &Engine{home: home, queue: queue, registry: registry, active: map[string]*projection.Projection{}, upgradePending: map[string]bool{}, notify: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}), lockFile: lockFile}
	if err := engine.recover(ctx); err != nil {
		engine.closeResources()
		return nil, err
	}
	go engine.worker()
	engine.signal()
	return engine, nil
}

func (e *Engine) recover(ctx context.Context) error {
	apps, err := e.registry.List(ctx)
	if err != nil {
		return err
	}
	for _, app := range apps {
		if app.ActiveRevision == nil {
			continue
		}
		sources, runtime, err := e.registry.RevisionSources(ctx, *app.ActiveRevision)
		if err != nil {
			return err
		}
		compiled, err := compile(app.Name, sources)
		if err != nil {
			return err
		}
		path := e.projectionPath(app.Name)
		var opened *projection.Projection
		if runtime != profile.RuntimeID {
			e.upgradePending[app.Name] = true
			opened, err = projection.OpenForUpgrade(ctx, path, compiled, *app.ActiveRevision)
		} else {
			opened, err = projection.Open(ctx, path, compiled)
		}
		if err != nil {
			return fmt.Errorf("recover projection %q: %w", app.Name, err)
		}
		e.active[app.Name] = opened
	}
	return nil
}

func (e *Engine) Close() error {
	close(e.stop)
	<-e.done
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.closeResources()
}
func (e *Engine) closeResources() error {
	var errs []error
	for _, item := range e.active {
		errs = append(errs, item.Close())
	}
	if e.registry != nil {
		errs = append(errs, e.registry.Close())
	}
	if e.queue != nil {
		errs = append(errs, e.queue.Close())
	}
	if e.lockFile != nil {
		_ = unix.Flock(int(e.lockFile.Fd()), unix.LOCK_UN)
		errs = append(errs, e.lockFile.Close())
	}
	return errors.Join(errs...)
}

func (e *Engine) Receiver() *ingest.Receiver {
	receiver := ingest.NewReceiver(e.queue, e.consumers, ingest.ReceiverLimits{})
	receiver.SetAcceptor(e.accept)
	receiver.SetAcceptedHook(e.signal)
	return receiver
}
func (e *Engine) consumers(ctx context.Context) ([]inbox.Consumer, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.consumersLocked(ctx)
}

func (e *Engine) consumersLocked(ctx context.Context) ([]inbox.Consumer, error) {
	if len(e.upgradePending) != 0 {
		return nil, ingest.ErrNotReady
	}
	names := sortedProjectionKeys(e.active)
	result := make([]inbox.Consumer, 0, len(names))
	for _, name := range names {
		frontier, err := e.active[name].Frontier(ctx)
		if err != nil {
			return nil, err
		}
		if frontier.Complete && frontier.GapPosition == nil {
			result = append(result, inbox.Consumer{Tailapp: name, Revision: e.active[name].Profile().Revision})
		}
	}
	return result, nil
}

func (e *Engine) accept(ctx context.Context, records []inbox.Record) ([]int64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	consumers, err := e.consumersLocked(ctx)
	if err != nil {
		return nil, err
	}
	return e.queue.Enqueue(ctx, records, consumers)
}
func (e *Engine) signal() {
	select {
	case e.notify <- struct{}{}:
	default:
	}
}
func (e *Engine) worker() {
	defer close(e.done)
	for {
		select {
		case <-e.stop:
			return
		case <-e.notify:
			_ = e.Drain(context.Background())
		}
	}
}

func (e *Engine) Drain(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.drainLocked(ctx)
}

func (e *Engine) drainLocked(ctx context.Context) error {
	if len(e.upgradePending) != 0 {
		return nil
	}
	for {
		type pending struct {
			name     string
			delivery inbox.Delivery
		}
		var wave []pending
		var minimum int64
		for _, name := range sortedProjectionKeys(e.active) {
			items, err := e.queue.Pending(ctx, name, 1)
			if err != nil {
				return err
			}
			if len(items) == 0 {
				continue
			}
			if minimum == 0 || items[0].Position < minimum {
				minimum = items[0].Position
				wave = nil
			}
			if items[0].Position == minimum {
				wave = append(wave, pending{name, items[0]})
			}
		}
		if len(wave) == 0 {
			return nil
		}
		for _, item := range wave {
			current := e.active[item.name]
			if current == nil {
				continue
			}
			_, err := current.Process(ctx, item.delivery)
			if err != nil {
				_ = e.queue.DetachAll(ctx, item.name, "projection_gap")
				continue
			}
			if err := e.queue.Complete(ctx, item.name, item.delivery.Position); err != nil {
				return err
			}
		}
	}
}

func (e *Engine) Apps(ctx context.Context) ([]definition.App, error) { return e.registry.List(ctx) }
func (e *Engine) App(ctx context.Context, name string) (definition.App, map[string][]byte, error) {
	app, err := e.registry.Get(ctx, name)
	if err != nil {
		return definition.App{}, nil, err
	}
	sources, err := e.registry.Sources(ctx, name)
	return app, sources, err
}
func (e *Engine) Create(ctx context.Context, name, bundle string) (definition.App, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := profile.ValidateName(name); err != nil {
		return definition.App{}, err
	}
	sources := map[string][]byte{}
	var err error
	if bundle != "" {
		sources, err = tailapps.Source(bundle)
		if err != nil {
			return definition.App{}, err
		}
	}
	return e.registry.Create(ctx, name, sources)
}
func (e *Engine) Put(ctx context.Context, name, path string, content []byte, expected string) (definition.App, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := profile.ValidateSourceElement(path, content); err != nil {
		return definition.App{}, err
	}
	return e.registry.Put(ctx, name, path, content, expected)
}
func (e *Engine) RemoveElement(ctx context.Context, name, path, expected string) (definition.App, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := profile.ValidateSourceElement(path, []byte("x")); err != nil {
		return definition.App{}, err
	}
	return e.registry.DeleteElement(ctx, name, path, expected)
}

func (e *Engine) Validate(ctx context.Context, name, expected string) (*profile.Profile, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	app, err := e.registry.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if app.DraftRevision != expected {
		return nil, definition.ErrRevisionChanged
	}
	sources, err := e.registry.Sources(ctx, name)
	if err != nil {
		return nil, err
	}
	compiled, err := compile(name, sources)
	if err != nil {
		return nil, err
	}
	if err := e.registry.RecordRevision(ctx, name, compiled.Revision, profile.RuntimeID, sources); err != nil {
		return nil, err
	}
	return compiled, nil
}

func (e *Engine) Activate(ctx context.Context, name, expected, mode string, ackReset bool) (projection.Frontier, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.drainLocked(ctx); err != nil {
		return projection.Frontier{}, err
	}
	app, err := e.registry.Get(ctx, name)
	if err != nil {
		return projection.Frontier{}, err
	}
	if app.DraftRevision != expected {
		return projection.Frontier{}, definition.ErrRevisionChanged
	}
	sources, err := e.registry.Sources(ctx, name)
	if err != nil {
		return projection.Frontier{}, err
	}
	compiled, err := compile(name, sources)
	if err != nil {
		return projection.Frontier{}, err
	}
	if err := e.registry.RecordRevision(ctx, name, compiled.Revision, profile.RuntimeID, sources); err != nil {
		return projection.Frontier{}, err
	}
	stats, err := e.queue.Stats(ctx)
	if err != nil {
		return projection.Frontier{}, err
	}
	boundary := stats.DeliveryHead
	current := e.active[name]
	if e.upgradePending[name] {
		if err := e.queue.DetachAll(ctx, name, "runtime_upgrade"); err != nil {
			return projection.Frontier{}, err
		}
	}
	switch mode {
	case "continue":
		if current == nil {
			return projection.Frontier{}, errors.New("first activation requires reset")
		}
		if err := current.Continue(ctx, compiled); err != nil {
			return projection.Frontier{}, err
		}
	case "reset":
		if !ackReset {
			return projection.Frontier{}, errors.New("reset acknowledgement required")
		}
		next, err := e.resetProjection(ctx, name, compiled, boundary)
		if err != nil {
			return projection.Frontier{}, err
		}
		current = next
	default:
		return projection.Frontier{}, errors.New("activation mode must be continue or reset")
	}
	if err := e.registry.Activate(ctx, name, compiled.Revision, profile.RuntimeID, mode, boundary, expected); err != nil {
		return projection.Frontier{}, err
	}
	e.active[name] = current
	delete(e.upgradePending, name)
	return current.Frontier(ctx)
}

func (e *Engine) resetProjection(ctx context.Context, name string, compiled *profile.Profile, boundary int64) (*projection.Projection, error) {
	directory := filepath.Join(e.home, "projections", name)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	stable := filepath.Join(directory, "state.sqlite")
	candidate := filepath.Join(directory, "state.candidate.sqlite")
	if _, err := os.Lstat(candidate); err == nil {
		return nil, errors.New("activation candidate already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	created, err := projection.Create(ctx, candidate, compiled, boundary, "reset")
	if err != nil {
		return nil, err
	}
	if err := created.Close(); err != nil {
		return nil, err
	}
	var backup string
	if current := e.active[name]; current != nil {
		_ = current.Close()
		backup = filepath.Join(directory, fmt.Sprintf("state.retired.%d.sqlite", time.Now().UnixNano()))
		if err := os.Rename(stable, backup); err != nil {
			return nil, err
		}
	}
	if err := os.Rename(candidate, stable); err != nil {
		if backup != "" {
			_ = os.Rename(backup, stable)
		}
		return nil, err
	}
	opened, err := projection.Open(ctx, stable, compiled)
	if err != nil {
		return nil, err
	}
	return opened, nil
}

func (e *Engine) Delete(ctx context.Context, name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if current := e.active[name]; current != nil {
		_ = e.queue.DetachAll(ctx, name, "tailapp_deleted")
		_ = current.Close()
		delete(e.active, name)
	}
	return e.registry.Delete(ctx, name)
}
func (e *Engine) Status(ctx context.Context) (Status, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	stats, err := e.queue.Stats(ctx)
	if err != nil {
		return Status{}, err
	}
	result := Status{Profile: profile.RuntimeID, IngestionReady: len(e.upgradePending) == 0, Inbox: stats, Apps: map[string]projection.Frontier{}}
	for name, item := range e.active {
		frontier, err := item.Frontier(ctx)
		if err != nil {
			return Status{}, err
		}
		result.Apps[name] = frontier
	}
	return result, nil
}

func (e *Engine) Query(ctx context.Context, app string, request query.Request, mountNames map[string]string) (query.Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	primary := e.active[app]
	if primary == nil {
		return query.Result{}, sql.ErrNoRows
	}
	frontier, err := primary.Frontier(ctx)
	if err != nil {
		return query.Result{}, err
	}
	mounts := map[string]query.Namespace{}
	for alias, name := range mountNames {
		item := e.active[name]
		if item == nil {
			return query.Result{}, fmt.Errorf("mount tailapp %q is absent", name)
		}
		mountedFrontier, err := item.Frontier(ctx)
		if err != nil {
			return query.Result{}, err
		}
		mounts[alias] = query.Namespace{Path: item.Path(), Profile: item.Profile(), Frontier: mountedFrontier}
	}
	sandbox, err := query.Open(query.Namespace{Path: primary.Path(), Profile: primary.Profile(), Frontier: frontier}, mounts)
	if err != nil {
		return query.Result{}, err
	}
	defer sandbox.Close()
	return sandbox.Query(ctx, request)
}

func (e *Engine) Schema(name string) (*profile.Profile, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	item := e.active[name]
	if item == nil {
		return nil, sql.ErrNoRows
	}
	return item.Profile(), nil
}

func (e *Engine) projectionPath(name string) string {
	return filepath.Join(e.home, "projections", name, "state.sqlite")
}
func compile(name string, sources map[string][]byte) (*profile.Profile, error) {
	files := fstest.MapFS{}
	for path, content := range sources {
		files[path] = &fstest.MapFile{Data: content, Mode: fs.FileMode(0o600)}
	}
	return profile.Load(files, ".", name)
}
func sortedProjectionKeys(values map[string]*projection.Projection) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
