package scripts_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/generalbusiness-ai/tailapps/internal/engine"
)

// The test owns and reaps its resident, just as the real service manager does.
// Sending TERM alone lets its replacement race the old engine's lock/socket.
type testService struct {
	*httptest.Server
	mu                     sync.Mutex
	command                func() *exec.Cmd
	cmd                    *exec.Cmd
	done                   chan struct{}
	logPath, failNextStart string
}

func newTestService(t *testing.T, logPath, failNextStart string, command func() *exec.Cmd) *testService {
	t.Helper()
	s := &testService{command: command, logPath: logPath, failNextStart: failNextStart}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.restart(); err != nil {
			t.Logf("test service restart failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	t.Cleanup(func() {
		s.Server.Close()
		s.mu.Lock()
		err := s.stop()
		s.mu.Unlock()
		if err != nil {
			t.Error(err)
		}
		if t.Failed() {
			if b, err := os.ReadFile(logPath); err == nil {
				t.Logf("All resident startup/exit attempts:\n%s", b)
			}
		}
	})
	return s
}

func (s *testService) restart() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.stop(); err != nil {
		return err
	}
	if s.failNextStart != "" {
		if err := os.Remove(s.failNextStart); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	log, err := os.OpenFile(s.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	cmd := s.command()
	cmd.Stdout, cmd.Stderr = log, log
	// Also stop descendants of the upgrade-pending shell stub at cleanup.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	fmt.Fprintf(log, "starting %s\n", cmd.Path)
	if err := cmd.Start(); err != nil {
		log.Close()
		return err
	}
	s.cmd, s.done = cmd, make(chan struct{})
	done := s.done
	go func() {
		err := cmd.Wait()
		fmt.Fprintf(log, "pid %d exited: %v\n", cmd.Process.Pid, err)
		log.Close()
		close(done)
	}()
	return nil
}

// Caller holds mu. A bounded failed stop refuses the restart; it never starts
// another resident while the old process might still own the home.
func (s *testService) stop() error {
	if s.cmd == nil {
		return nil
	}
	select {
	case <-s.done:
		s.cmd = nil
		return nil
	default:
	}
	_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-s.done:
		s.cmd = nil
		return nil
	case <-time.After(15 * time.Second):
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		<-s.done
		s.cmd = nil
		return errors.New("test resident did not exit after TERM; killed and reaped")
	}
}

// A controlled resident holds the real engine lock after TERM until the test
// releases its pipe. No scheduler timing is needed to force shutdown overlap.
func TestServiceResidentProcess(t *testing.T) {
	if os.Getenv("TAILAPPS_SERVICE_HELPER") != "1" {
		t.Skip("resident subprocess helper")
	}
	resident, err := engine.Open(context.Background(), os.Getenv("TAILAPP_HOME"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer resident.Close()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM)
	defer signal.Stop(stop)
	ready := os.Getenv("TAILAPPS_TEST_READY")
	if err := os.WriteFile(ready, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	<-stop
	if os.Getenv("TAILAPPS_TEST_HOLD_EXIT") == "1" {
		if err := os.WriteFile(ready+".stopping", nil, 0o600); err != nil {
			t.Fatal(err)
		}
		gate := os.NewFile(3, "exit-gate")
		var b [1]byte
		_, _ = gate.Read(b[:])
		gate.Close()
	}
}

func TestServiceRestartWaitsForResidentExit(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	gate, release, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	defer release.Close() // unblocks the old child even if a regression fails
	generation := 0
	service := newTestService(t, filepath.Join(home, "resident.log"), "", func() *exec.Cmd {
		generation++
		cmd := exec.Command(executable, "-test.run=^TestServiceResidentProcess$")
		cmd.Env = append(os.Environ(), "TAILAPPS_SERVICE_HELPER=1", "TAILAPP_HOME="+home,
			fmt.Sprintf("TAILAPPS_TEST_READY=%s/ready-%d", home, generation))
		if generation == 1 {
			cmd.ExtraFiles = []*os.File{gate}
			cmd.Env = append(cmd.Env, "TAILAPPS_TEST_HOLD_EXIT=1")
		}
		return cmd
	})
	restart := func() error {
		client := &http.Client{Timeout: 20 * time.Second}
		response, err := client.Get(service.URL)
		if err != nil {
			return err
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("restart status %s", response.Status)
		}
		return nil
	}
	if err := restart(); err != nil {
		t.Fatal(err)
	}
	awaitServiceFile(t, filepath.Join(home, "ready-1"))
	finished := make(chan error, 1)
	go func() { finished <- restart() }()
	awaitServiceFile(t, filepath.Join(home, "ready-1.stopping"))
	if resident, err := engine.Open(context.Background(), home); err == nil {
		resident.Close()
		t.Fatal("controlled old resident lost its engine lock before exit")
	} else if err.Error() != "engine_already_running" {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		t.Fatalf("restart returned before old resident exit: %v", err)
	default:
	}
	if _, err := release.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("restart did not finish after old resident exit")
	}
	awaitServiceFile(t, filepath.Join(home, "ready-2"))
}

func awaitServiceFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("resident did not reach controlled boundary %s", path)
}
