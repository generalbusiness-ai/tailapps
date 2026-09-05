// Package engine coordinates definition lifecycle, ingestion obligations,
// flat projection waves and aligned query snapshots for one resident process.
package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing/fstest"
	"time"

	"golang.org/x/sys/unix"

	"github.com/generalbusiness-ai/tailapps/internal/definition"
	"github.com/generalbusiness-ai/tailapps/internal/inbox"
	"github.com/generalbusiness-ai/tailapps/internal/ingest"
	operationalmetrics "github.com/generalbusiness-ai/tailapps/internal/metrics"
	"github.com/generalbusiness-ai/tailapps/internal/profile"
	"github.com/generalbusiness-ai/tailapps/internal/projection"
	"github.com/generalbusiness-ai/tailapps/internal/query"
	"github.com/generalbusiness-ai/tailapps/tailapps"
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
	unavailable    map[string]string
	metrics        *operationalmetrics.Registry
	processing     map[string]*operationalmetrics.Processing
	ineffective    map[string][]IneffectiveRecord
}

var ErrProjectionUnavailable = errors.New("projection_unavailable")

type Status struct {
	Profile        string                         `json:"profile"`
	IngestionReady bool                           `json:"ingestion_ready"`
	Inbox          inbox.Stats                    `json:"inbox"`
	Apps           map[string]projection.Frontier `json:"apps"`
	Unavailable    map[string]string              `json:"unavailable,omitempty"`
}

type InstallResult struct {
	App      definition.App      `json:"app"`
	Profile  *profile.Profile    `json:"profile"`
	Frontier projection.Frontier `json:"frontier"`
}

const (
	MaxMetricTailapps            = 256
	IneffectiveBufferCapacity    = 16
	MaxIneffectiveRecordJSONSize = 32 << 10
)

type IneffectiveRecord struct {
	Position         int64           `json:"position"`
	EventID          string          `json:"event_id"`
	Revision         string          `json:"revision"`
	Signal           string          `json:"signal"`
	Name             string          `json:"name"`
	Source           string          `json:"source"`
	TimeUnixNano     *string         `json:"time_unix_nano,omitempty"`
	ObservedUnixNano *string         `json:"observed_unix_nano,omitempty"`
	ReceivedUnixNano string          `json:"received_at_unix_nano"`
	TraceID          *string         `json:"trace_id,omitempty"`
	SpanID           *string         `json:"span_id,omitempty"`
	ContentDigest    string          `json:"content_digest"`
	RecordBytes      int             `json:"record_bytes"`
	RecordOmitted    bool            `json:"record_omitted,omitempty"`
	Record           json.RawMessage `json:"record,omitempty"`
}

type IneffectiveSnapshot struct {
	Tailapp            string              `json:"tailapp"`
	Revision           string              `json:"revision"`
	Capacity           int                 `json:"capacity"`
	IneffectiveRecords int64               `json:"ineffective_records"`
	AvailableRecords   int                 `json:"available_records"`
	UnavailableRecords int64               `json:"unavailable_records"`
	Records            []IneffectiveRecord `json:"records"`
}

type TailappMetrics struct {
	DeliveryHead        int64            `json:"delivery_head"`
	InterpretedPosition int64            `json:"interpreted_position"`
	LagPositions        int64            `json:"lag_positions"`
	Complete            bool             `json:"complete"`
	GapPosition         *int64           `json:"gap_position,omitempty"`
	GapObservedUnixNano *string          `json:"gap_observed_unix_nano,omitempty"`
	LastRecordUnixNano  *string          `json:"last_record_time_unix_nano,omitempty"`
	Durable             projection.Stats `json:"durable"`
}

type MetricsSnapshot struct {
	operationalmetrics.Snapshot
	Inbox                      inbox.Stats               `json:"inbox"`
	OldestInboxAgeMilliseconds *float64                  `json:"oldest_inbox_age_milliseconds"`
	Tailapps                   map[string]TailappMetrics `json:"tailapps"`
	ActiveTailapps             int                       `json:"active_tailapps"`
	UnavailableTailapps        int                       `json:"unavailable_tailapps"`
	UpgradePendingTailapps     int                       `json:"upgrade_pending_tailapps"`
	OmittedTailapps            int                       `json:"omitted_tailapps"`
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
	engine := &Engine{home: home, queue: queue, registry: registry, active: map[string]*projection.Projection{}, upgradePending: map[string]bool{}, unavailable: map[string]string{}, metrics: operationalmetrics.New(), processing: map[string]*operationalmetrics.Processing{}, ineffective: map[string][]IneffectiveRecord{}, notify: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}), lockFile: lockFile}
	if err := engine.recoverActivations(ctx); err != nil {
		engine.closeResources()
		return nil, err
	}
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
		if _, blocked := e.unavailable[app.Name]; blocked {
			continue
		}
		e.cleanupActivationFiles(app.Name)
		if app.ActiveRevision == nil {
			continue
		}
		sources, runtime, err := e.registry.RevisionSources(ctx, *app.ActiveRevision)
		if err != nil {
			e.unavailable[app.Name] = err.Error()
			continue
		}
		// Historical identities are retained for query-only recognition, not replay
		// with historical evaluation semantics. Every non-current runtime stays
		// upgrade-pending until an acknowledged reset creates a fresh projection.
		var compiled *profile.Profile
		if runtime == profile.RuntimeID {
			compiled, err = legacyCompile(app.Name, sources)
		} else {
			compiled, err = compile(app.Name, sources)
		}
		if err != nil {
			e.unavailable[app.Name] = err.Error()
			continue
		}
		path := e.projectionPath(app.Name)
		var opened *projection.Projection
		if runtime != profile.CurrentRuntimeID() {
			e.upgradePending[app.Name] = true
			opened, err = projection.OpenForUpgrade(ctx, path, compiled, *app.ActiveRevision)
		} else {
			opened, err = projection.Open(ctx, path, compiled)
		}
		if err != nil {
			e.unavailable[app.Name] = fmt.Sprintf("recover projection: %v", err)
			delete(e.upgradePending, app.Name)
			continue
		}
		e.active[app.Name] = opened
		e.processing[app.Name] = operationalmetrics.NewProcessing()
	}
	return nil
}

func (e *Engine) recoverActivations(ctx context.Context) error {
	journals, err := e.registry.ActivationJournals(ctx)
	if err != nil {
		return err
	}
	for _, journal := range journals {
		stable, candidate, previous := e.activationPaths(journal.Name)
		identity, identityErr := projection.InspectIdentity(ctx, stable)
		if identityErr == nil && identity.Name == journal.Name && identity.Revision == journal.NewRevision && identity.Runtime == journal.Runtime {
			if err := e.registry.FinishActivation(ctx, journal); err != nil {
				e.unavailable[journal.Name] = fmt.Sprintf("finish activation journal: %v", err)
				continue
			}
			removeProjectionFile(candidate)
			removeProjectionFile(previous)
			continue
		}
		// The projection pointer never reached the new complete identity. Roll
		// back to the previous complete file when present; otherwise the old
		// stable file remains authoritative.
		if _, err := os.Lstat(previous); err == nil {
			removeProjectionFile(stable)
			if err := os.Rename(previous, stable); err != nil {
				e.unavailable[journal.Name] = fmt.Sprintf("restore activation journal: %v", err)
				continue
			}
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			e.unavailable[journal.Name] = fmt.Sprintf("inspect activation backup: %v", err)
			continue
		}
		removeProjectionFile(candidate)
		if err := e.registry.AbortActivation(ctx, journal.Name, journal.NewRevision); err != nil {
			e.unavailable[journal.Name] = fmt.Sprintf("abort activation journal: %v", err)
		}
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
	receiver.SetMetrics(e.metrics)
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
	positions, err := e.queue.Enqueue(ctx, records, consumers)
	if err == nil {
		recordsBySignal := map[string]int{}
		for _, record := range records {
			recordsBySignal[record.Signal]++
		}
		e.metrics.ObserveRouting(recordsBySignal, len(records)*len(consumers))
	}
	return positions, err
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
			e.mu.Lock()
			more, _ := e.drainPassLocked(context.Background(), 16)
			e.mu.Unlock()
			if more {
				e.signal()
			}
		}
	}
}

func (e *Engine) Drain(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.drainLocked(ctx)
}

func (e *Engine) drainLocked(ctx context.Context) error {
	_, err := e.drainPassLocked(ctx, 0)
	return err
}

// drainPassLocked processes complete flat waves. A positive wave limit lets
// the resident worker yield the engine lock between bounded passes so status,
// query and shutdown can reach the barrier even under sustained backlog.
func (e *Engine) drainPassLocked(ctx context.Context, waveLimit int) (bool, error) {
	if len(e.upgradePending) != 0 {
		return false, nil
	}
	waves := 0
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
				return false, err
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
			return false, nil
		}
		for _, item := range wave {
			current := e.active[item.name]
			if current == nil {
				continue
			}
			started := time.Now()
			queueDelay := time.Duration(0)
			if !item.delivery.ReceivedAt.IsZero() {
				queueDelay = e.wallElapsed(item.delivery.ReceivedAt)
			}
			processed, err := current.Process(ctx, item.delivery)
			if err != nil {
				// Process records only deterministic application failures as a
				// gap. Cancellation and transient storage errors leave both the
				// frontier and obligation untouched so a later pass can retry.
				frontier, frontierErr := current.Frontier(context.Background())
				if frontierErr != nil {
					e.processing[item.name].Observe(0, false, "error", queueDelay, time.Since(started))
					return false, errors.Join(err, frontierErr)
				}
				if frontier.GapPosition == nil {
					e.processing[item.name].Observe(0, false, "retry", queueDelay, time.Since(started))
					return false, err
				}
				detached, detachErr := e.queue.DetachAll(context.Background(), item.name, "projection_gap")
				if detachErr != nil {
					e.processing[item.name].Observe(0, false, "error", queueDelay, time.Since(started))
					return false, detachErr
				}
				e.metrics.ObserveDetachedObligations("projection_gap", detached)
				e.processing[item.name].ObserveDetached("projection_gap", detached)
				e.processing[item.name].Observe(0, false, "gap", queueDelay, time.Since(started))
				continue
			}
			if processed.Ineffective {
				e.recordIneffectiveLocked(item.name, item.delivery)
			}
			if err := e.queue.Complete(ctx, item.name, item.delivery.Position); err != nil {
				e.processing[item.name].Observe(processed.EmittedEvents, processed.Ineffective, "retry", queueDelay, time.Since(started))
				return false, err
			}
			e.processing[item.name].Observe(processed.EmittedEvents, processed.Ineffective, "ok", queueDelay, time.Since(started))
		}
		waves++
		if waveLimit > 0 && waves >= waveLimit {
			return true, nil
		}
	}
}

func (e *Engine) Apps(ctx context.Context) ([]definition.App, error) { return e.registry.List(ctx) }

// BeginMutation and CompleteMutation expose the control database's durable
// idempotency ledger. The control server serializes the bind/effect/complete
// sequence; engine mutation methods retain their own lifecycle lock.
func (e *Engine) BeginMutation(ctx context.Context, key, operation, requestDigest string) (definition.MutationRecord, bool, error) {
	return e.registry.BeginMutation(ctx, key, operation, requestDigest)
}

func (e *Engine) CompleteMutation(ctx context.Context, key, operation, requestDigest string, record definition.MutationRecord) error {
	return e.registry.CompleteMutation(ctx, key, operation, requestDigest, record)
}

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

// Install validates a complete source set and first-activates it as one
// create-only operation. Incremental updates keep using the explicit
// draft/revision lifecycle so this convenience boundary cannot silently
// replace or reset an existing Tailapp.
func (e *Engine) Install(ctx context.Context, name, bundle string, sources map[string][]byte) (InstallResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := profile.ValidateName(name); err != nil {
		return InstallResult{}, err
	}
	if bundle != "" && len(sources) != 0 {
		return InstallResult{}, errors.New("install accepts either bundle or sources, not both")
	}
	if bundle != "" {
		var err error
		sources, err = tailapps.Source(bundle)
		if err != nil {
			return InstallResult{}, err
		}
	} else if len(sources) == 0 {
		return InstallResult{}, errors.New("install requires a bundle or complete sources")
	}
	compiled, err := compile(name, sources)
	if err != nil {
		return InstallResult{}, err
	}
	if err := e.drainLocked(ctx); err != nil {
		return InstallResult{}, err
	}
	if _, err := e.registry.Get(ctx, name); err == nil {
		return InstallResult{}, fmt.Errorf("tailapp %q already exists", name)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return InstallResult{}, err
	}
	app, err := e.registry.Create(ctx, name, sources)
	if err != nil {
		return InstallResult{}, err
	}
	frontier, err := e.activateLocked(ctx, name, app.DraftRevision, "reset", true)
	if err != nil {
		return InstallResult{}, fmt.Errorf("activate installed tailapp (validated draft remains): %w", err)
	}
	app, err = e.registry.Get(ctx, name)
	if err != nil {
		return InstallResult{}, err
	}
	return InstallResult{App: app, Profile: compiled, Frontier: frontier}, nil
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
	if err := e.registry.RecordRevision(ctx, name, compiled.Revision, compiled.RuntimeProfile, sources); err != nil {
		return nil, err
	}
	return compiled, nil
}

func (e *Engine) Activate(ctx context.Context, name, expected, mode string, ackReset bool) (projection.Frontier, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.activateLocked(ctx, name, expected, mode, ackReset)
}

func (e *Engine) activateLocked(ctx context.Context, name, expected, mode string, ackReset bool) (projection.Frontier, error) {
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
	if err := e.registry.RecordRevision(ctx, name, compiled.Revision, compiled.RuntimeProfile, sources); err != nil {
		return projection.Frontier{}, err
	}
	stats, err := e.queue.Stats(ctx)
	if err != nil {
		return projection.Frontier{}, err
	}
	boundary := stats.DeliveryHead
	current := e.active[name]
	journal := definition.ActivationJournal{Name: name, NewRevision: compiled.Revision, Runtime: compiled.RuntimeProfile, Mode: mode, Boundary: boundary, ExpectedDraft: expected, OldRevision: app.ActiveRevision}
	switch mode {
	case "continue":
		if current == nil {
			if _, blocked := e.unavailable[name]; blocked {
				return projection.Frontier{}, fmt.Errorf("%w: reset is required", ErrProjectionUnavailable)
			}
			return projection.Frontier{}, errors.New("first activation requires reset")
		}
		if err := profile.ContinueCompatible(current.Profile(), compiled); err != nil {
			return projection.Frontier{}, err
		}
		storedRuntime, err := current.StoredRuntime(ctx)
		if err != nil {
			return projection.Frontier{}, err
		}
		if storedRuntime != compiled.RuntimeProfile {
			return projection.Frontier{}, errors.New("stored runtime profile changed; acknowledged reset is required")
		}
		frontier, err := current.Frontier(ctx)
		if err != nil {
			return projection.Frontier{}, err
		}
		// A healthy current revision must have reached the activation barrier:
		// drainLocked ran under this same engine lock. Gapped revisions and
		// runtime upgrades are explicit skip/repair paths and may trail it.
		if frontier.GapPosition == nil && !e.upgradePending[name] && frontier.InterpretedPosition != boundary {
			return projection.Frontier{}, errors.New("activation boundary is not fully drained")
		}
	case "reset":
		if !ackReset {
			return projection.Frontier{}, errors.New("reset acknowledgement required")
		}
	default:
		return projection.Frontier{}, errors.New("activation mode must be continue or reset")
	}
	if err := e.registry.BeginActivation(ctx, journal); err != nil {
		return projection.Frontier{}, err
	}
	abort := func() { _ = e.registry.AbortActivation(context.Background(), name, compiled.Revision) }
	if e.upgradePending[name] {
		detached, err := e.queue.DetachAll(ctx, name, "runtime_upgrade")
		if err != nil {
			abort()
			return projection.Frontier{}, err
		}
		e.metrics.ObserveDetachedObligations("runtime_upgrade", detached)
		if handle := e.processing[name]; handle != nil {
			handle.ObserveDetached("runtime_upgrade", detached)
		}
	}
	if mode == "continue" {
		if err := current.Continue(ctx, compiled, boundary); err != nil {
			abort()
			return projection.Frontier{}, err
		}
	} else {
		next, err := e.resetProjection(ctx, name, compiled, boundary)
		if err != nil {
			abort()
			return projection.Frontier{}, err
		}
		current = next
	}
	if err := e.registry.FinishActivation(ctx, journal); err != nil {
		_ = current.Close()
		delete(e.active, name)
		delete(e.processing, name)
		delete(e.ineffective, name)
		e.unavailable[name] = fmt.Sprintf("finish activation: %v", err)
		return projection.Frontier{}, fmt.Errorf("%w: activation journal awaits recovery", ErrProjectionUnavailable)
	}
	delete(e.ineffective, name)
	_, _, previous := e.activationPaths(name)
	removeProjectionFile(previous)
	e.active[name] = current
	if e.processing[name] == nil {
		e.processing[name] = operationalmetrics.NewProcessing()
	}
	delete(e.upgradePending, name)
	delete(e.unavailable, name)
	return current.Frontier(ctx)
}

func (e *Engine) resetProjection(ctx context.Context, name string, compiled *profile.Profile, boundary int64) (*projection.Projection, error) {
	directory := filepath.Join(e.home, "projections", name)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	stable, candidate, backup := e.activationPaths(name)
	removeProjectionFile(candidate)
	removeProjectionFile(backup)
	created, err := projection.Create(ctx, candidate, compiled, boundary, "reset")
	if err != nil {
		return nil, err
	}
	if err := created.Close(); err != nil {
		removeProjectionFile(candidate)
		return nil, err
	}
	current := e.active[name]
	var currentProfile *profile.Profile
	if current != nil {
		currentProfile = current.Profile()
		if err := current.Close(); err != nil {
			removeProjectionFile(candidate)
			return nil, err
		}
	}
	stableMoved := false
	if _, err := os.Lstat(stable); err == nil {
		if err := os.Rename(stable, backup); err != nil {
			e.reopenAfterResetFailure(ctx, name, stable, currentProfile)
			removeProjectionFile(candidate)
			return nil, err
		}
		stableMoved = true
	} else if !errors.Is(err, os.ErrNotExist) {
		e.reopenAfterResetFailure(ctx, name, stable, currentProfile)
		removeProjectionFile(candidate)
		return nil, err
	}
	if err := os.Rename(candidate, stable); err != nil {
		if stableMoved {
			_ = os.Rename(backup, stable)
		}
		e.reopenAfterResetFailure(ctx, name, stable, currentProfile)
		removeProjectionFile(candidate)
		return nil, err
	}
	opened, err := projection.Open(ctx, stable, compiled)
	if err != nil {
		removeProjectionFile(stable)
		if stableMoved {
			_ = os.Rename(backup, stable)
		}
		e.reopenAfterResetFailure(ctx, name, stable, currentProfile)
		return nil, err
	}
	return opened, nil
}

func (e *Engine) reopenAfterResetFailure(ctx context.Context, name, stable string, compiled *profile.Profile) {
	if compiled == nil {
		return
	}
	if reopened, err := projection.Open(ctx, stable, compiled); err == nil {
		e.active[name] = reopened
		if e.processing[name] == nil {
			e.processing[name] = operationalmetrics.NewProcessing()
		}
	}
}

func (e *Engine) activationPaths(name string) (stable, candidate, previous string) {
	directory := filepath.Join(e.home, "projections", name)
	return filepath.Join(directory, "state.sqlite"), filepath.Join(directory, "state.candidate.sqlite"), filepath.Join(directory, "state.previous.sqlite")
}

func (e *Engine) cleanupActivationFiles(name string) {
	_, candidate, previous := e.activationPaths(name)
	removeProjectionFile(candidate)
	removeProjectionFile(previous)
}

func removeProjectionFile(path string) {
	for _, target := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return
		}
	}
}

func (e *Engine) Delete(ctx context.Context, name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if current := e.active[name]; current != nil {
		detached, _ := e.queue.DetachAll(ctx, name, "tailapp_deleted")
		e.metrics.ObserveDetachedObligations("tailapp_deleted", detached)
		if handle := e.processing[name]; handle != nil {
			handle.ObserveDetached("tailapp_deleted", detached)
		}
		_ = current.Close()
		delete(e.active, name)
		delete(e.processing, name)
	}
	delete(e.ineffective, name)
	delete(e.unavailable, name)
	delete(e.upgradePending, name)
	return e.registry.Delete(ctx, name)
}

func (e *Engine) recordIneffectiveLocked(name string, delivery inbox.Delivery) {
	if e.ineffective == nil {
		e.ineffective = map[string][]IneffectiveRecord{}
	}
	receivedAt := delivery.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = time.Now()
	}
	record := IneffectiveRecord{
		Position: delivery.Position, EventID: delivery.EventID, Revision: delivery.Revision,
		Signal: delivery.Signal, Name: delivery.Name, Source: delivery.Source,
		TimeUnixNano: delivery.TimeUnixNano, ObservedUnixNano: delivery.ObservedUnixNano,
		ReceivedUnixNano: strconv.FormatInt(receivedAt.UnixNano(), 10),
		TraceID:          delivery.TraceID, SpanID: delivery.SpanID, ContentDigest: delivery.ContentDigest,
		RecordBytes: len(delivery.JSON),
	}
	if len(delivery.JSON) <= MaxIneffectiveRecordJSONSize {
		record.Record = append(json.RawMessage(nil), delivery.JSON...)
	} else {
		record.RecordOmitted = true
	}
	items := append(e.ineffective[name], record)
	if len(items) > IneffectiveBufferCapacity {
		items = append([]IneffectiveRecord(nil), items[len(items)-IneffectiveBufferCapacity:]...)
	}
	e.ineffective[name] = items
}

func (e *Engine) Ineffective(ctx context.Context, name string) (IneffectiveSnapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	current := e.active[name]
	if current == nil {
		if _, blocked := e.unavailable[name]; blocked {
			return IneffectiveSnapshot{}, fmt.Errorf("%w: %s", ErrProjectionUnavailable, name)
		}
		return IneffectiveSnapshot{}, sql.ErrNoRows
	}
	durable, err := current.Stats(ctx)
	if err != nil {
		return IneffectiveSnapshot{}, err
	}
	items := e.ineffective[name]
	unavailable := durable.IneffectiveRecords - int64(len(items))
	if unavailable < 0 {
		unavailable = 0
	}
	result := IneffectiveSnapshot{
		Tailapp: name, Revision: current.Profile().Revision, Capacity: IneffectiveBufferCapacity,
		IneffectiveRecords: durable.IneffectiveRecords, AvailableRecords: len(items), UnavailableRecords: unavailable,
		Records: make([]IneffectiveRecord, len(items)),
	}
	copy(result.Records, items)
	for index := range result.Records {
		result.Records[index].Record = append(json.RawMessage(nil), items[index].Record...)
	}
	return result, nil
}
func (e *Engine) Status(ctx context.Context) (Status, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	stats, err := e.queue.Stats(ctx)
	if err != nil {
		return Status{}, err
	}
	result := Status{Profile: profile.CurrentRuntimeID(), IngestionReady: len(e.upgradePending) == 0, Inbox: stats, Apps: map[string]projection.Frontier{}, Unavailable: map[string]string{}}
	for name, item := range e.active {
		frontier, err := item.Frontier(ctx)
		if err != nil {
			return Status{}, err
		}
		result.Apps[name] = frontier
	}
	for name, reason := range e.unavailable {
		result.Unavailable[name] = reason
	}
	return result, nil
}

func (e *Engine) Metrics(ctx context.Context) (MetricsSnapshot, error) {
	started := time.Now()
	e.mu.Lock()
	stats, err := e.queue.Stats(ctx)
	if err != nil {
		e.mu.Unlock()
		return MetricsSnapshot{}, err
	}
	names := sortedProjectionKeys(e.active)
	activeCount := len(names)
	omitted := 0
	if len(names) > MaxMetricTailapps {
		omitted = len(names) - MaxMetricTailapps
		names = names[:MaxMetricTailapps]
	}
	handles := make(map[string]*operationalmetrics.Processing, len(names))
	tailappMetrics := make(map[string]TailappMetrics, len(names))
	for _, name := range names {
		item := e.active[name]
		frontier, err := item.Frontier(ctx)
		if err != nil {
			e.mu.Unlock()
			return MetricsSnapshot{}, err
		}
		durable, err := item.Stats(ctx)
		if err != nil {
			e.mu.Unlock()
			return MetricsSnapshot{}, err
		}
		lag := stats.DeliveryHead - frontier.InterpretedPosition
		if lag < 0 {
			lag = 0
		}
		tailappMetrics[name] = TailappMetrics{DeliveryHead: stats.DeliveryHead, InterpretedPosition: frontier.InterpretedPosition, LagPositions: lag, Complete: frontier.Complete, GapPosition: frontier.GapPosition, GapObservedUnixNano: frontier.GapObservedUnixNano, LastRecordUnixNano: frontier.LastRecordUnixNano, Durable: durable}
		if handle := e.processing[name]; handle != nil {
			handles[name] = handle
		}
	}
	unavailableCount := len(e.unavailable)
	upgradeCount := len(e.upgradePending)
	e.mu.Unlock()
	result := MetricsSnapshot{Snapshot: e.metrics.Snapshot(handles), Inbox: stats, Tailapps: tailappMetrics, ActiveTailapps: activeCount, UnavailableTailapps: unavailableCount, UpgradePendingTailapps: upgradeCount, OmittedTailapps: omitted}
	if stats.OldestReceivedAtUnixNano != nil {
		elapsed := e.wallElapsed(time.Unix(0, *stats.OldestReceivedAtUnixNano))
		age := float64(elapsed/time.Microsecond) / 1000
		result.OldestInboxAgeMilliseconds = &age
	}
	result.SnapshotDurationMilliseconds = float64(time.Since(started)/time.Microsecond) / 1000
	return result, nil
}

func (e *Engine) wallElapsed(since time.Time) time.Duration {
	elapsed := time.Since(since)
	if elapsed < 0 {
		e.metrics.ObserveClockRegression()
		return 0
	}
	return elapsed
}

func (e *Engine) ObserveControl(operation, outcome string, duration time.Duration) {
	e.metrics.ObserveControl(operation, outcome, duration)
}

func (e *Engine) Query(ctx context.Context, app string, request query.Request, mountNames map[string]string) (result query.Result, resultErr error) {
	started := time.Now()
	lockStarted := time.Now()
	e.mu.Lock()
	lockWait := time.Since(lockStarted)
	defer func() {
		e.mu.Unlock()
		e.metrics.ObserveQuery(len(result.Rows), result.ResultBytes, result.Truncated, queryMetricOutcome(resultErr), lockWait, time.Since(started))
	}()
	primary := e.active[app]
	if primary == nil {
		if _, blocked := e.unavailable[app]; blocked {
			return query.Result{}, fmt.Errorf("%w: %s", ErrProjectionUnavailable, app)
		}
		return query.Result{}, sql.ErrNoRows
	}
	frontier, err := primary.Frontier(ctx)
	if err != nil {
		return query.Result{}, err
	}
	durable, err := primary.Stats(ctx)
	if err != nil {
		return query.Result{}, err
	}
	stats, err := e.queue.Stats(ctx)
	if err != nil {
		return query.Result{}, err
	}
	mounts := map[string]query.Namespace{}
	for alias, name := range mountNames {
		item := e.active[name]
		if item == nil {
			if _, blocked := e.unavailable[name]; blocked {
				return query.Result{}, fmt.Errorf("%w: mount %s", ErrProjectionUnavailable, name)
			}
			return query.Result{}, fmt.Errorf("mount tailapp %q is absent", name)
		}
		mountedFrontier, err := item.Frontier(ctx)
		if err != nil {
			return query.Result{}, err
		}
		mounts[alias] = query.Namespace{Path: item.Path(), Profile: item.Profile(), Frontier: mountedFrontier}
	}
	sandbox, err := query.Open(query.Namespace{Path: primary.Path(), Profile: primary.Profile(), Frontier: frontier,
		DeliveryHead: stats.DeliveryHead, IneffectiveRecords: durable.IneffectiveRecords}, mounts)
	if err != nil {
		return query.Result{}, err
	}
	defer sandbox.Close()
	return sandbox.Query(ctx, request)
}

func queryMetricOutcome(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, sql.ErrNoRows):
		return "not_found"
	case errors.Is(err, ErrProjectionUnavailable):
		return "unavailable"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case strings.Contains(err.Error(), "query_budget_exceeded"):
		return "budget"
	case strings.Contains(err.Error(), "frontier_changed"):
		return "frontier_changed"
	default:
		return "error"
	}
}

func (e *Engine) Schema(name string) (*profile.Profile, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	item := e.active[name]
	if item == nil {
		if _, blocked := e.unavailable[name]; blocked {
			return nil, fmt.Errorf("%w: %s", ErrProjectionUnavailable, name)
		}
		return nil, sql.ErrNoRows
	}
	return item.Profile(), nil
}

func (e *Engine) projectionPath(name string) string {
	return filepath.Join(e.home, "projections", name, "state.sqlite")
}

// compile is the live compilation path: the extracted core under the
// composed runtime identity.
func compile(name string, sources map[string][]byte) (*profile.Profile, error) {
	files := fstest.MapFS{}
	for path, content := range sources {
		files[path] = &fstest.MapFile{Data: content, Mode: fs.FileMode(0o600)}
	}
	return profile.LoadCurrent(files, ".", name)
}

// legacyCompile is the retained resolver for revisions recorded under the
// legacy RuntimeID: the pre-extraction implementation with the legacy
// identity seed, so an old active projection's revision digest still
// matches and it remains queryable without silent reinterpretation.
func legacyCompile(name string, sources map[string][]byte) (*profile.Profile, error) {
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
