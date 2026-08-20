package update

import (
	"errors"
	"io"
	"log/slog"
	"os/exec"
	"testing"
	"time"
)

var errTest = errors.New("kickstart failed")

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// The dashboard's update path (POST /api/update/apply -> Restart) must hand the
// restart to launchd when the agent owns the process. Spawning a replacement
// instead orphans the job: launchd believes it is stopped, so nothing restarts
// onWatch after a later crash.
func TestRestartPrefersLaunchdWhenAgentOwnsProcess(t *testing.T) {
	restarts := 0
	spawned := 0
	exited := 0

	prevSup, prevInst, prevLoad, prevRestart := launchdSupported, launchdInstalled, launchdLoaded, launchdRestart
	prevExec, prevExit, prevSleep := execCommand, exitFn, sleepFn
	t.Cleanup(func() {
		launchdSupported, launchdInstalled, launchdLoaded, launchdRestart = prevSup, prevInst, prevLoad, prevRestart
		execCommand, exitFn, sleepFn = prevExec, prevExit, prevSleep
	})

	launchdSupported = func() bool { return true }
	launchdInstalled = func() bool { return true }
	launchdLoaded = func() bool { return true }
	launchdRestart = func() error { restarts++; return nil }
	execCommand = func(name string, args ...string) *exec.Cmd { spawned++; return prevExec("true") }
	exitFn = func(int) { exited++ }
	sleepFn = func(time.Duration) {}

	u := NewUpdater("1.0.0", quietLogger())
	if err := u.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}

	if restarts != 1 {
		t.Errorf("expected one launchctl kickstart, got %d", restarts)
	}
	if spawned != 0 {
		t.Errorf("launchd owns the process - must not spawn a replacement, spawned=%d", spawned)
	}
	if exited != 1 {
		t.Errorf("expected the old process to exit, exited=%d", exited)
	}
}

// If launchctl fails, the restart must still happen the old way.
func TestRestartFallsBackWhenLaunchctlFails(t *testing.T) {
	spawned := 0

	prevSup, prevInst, prevLoad, prevRestart := launchdSupported, launchdInstalled, launchdLoaded, launchdRestart
	prevExec, prevExit, prevSleep := execCommand, exitFn, sleepFn
	t.Cleanup(func() {
		launchdSupported, launchdInstalled, launchdLoaded, launchdRestart = prevSup, prevInst, prevLoad, prevRestart
		execCommand, exitFn, sleepFn = prevExec, prevExit, prevSleep
	})

	launchdSupported = func() bool { return true }
	launchdInstalled = func() bool { return true }
	launchdLoaded = func() bool { return true }
	launchdRestart = func() error { return errTest }
	execCommand = func(name string, args ...string) *exec.Cmd { spawned++; return prevExec("true") }
	exitFn = func(int) {}
	sleepFn = func(time.Duration) {}

	u := NewUpdater("1.0.0", quietLogger())
	if err := u.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if spawned == 0 {
		t.Error("expected a spawn fallback after launchctl failed")
	}
}

// With no agent installed, nothing about the old standalone path changes.
func TestRestartUnchangedWithoutLaunchdAgent(t *testing.T) {
	restarts := 0
	spawned := 0

	prevSup, prevRestart := launchdSupported, launchdRestart
	prevExec, prevExit, prevSleep := execCommand, exitFn, sleepFn
	t.Cleanup(func() {
		launchdSupported, launchdRestart = prevSup, prevRestart
		execCommand, exitFn, sleepFn = prevExec, prevExit, prevSleep
	})

	launchdSupported = func() bool { return false }
	launchdRestart = func() error { restarts++; return nil }
	execCommand = func(name string, args ...string) *exec.Cmd { spawned++; return prevExec("true") }
	exitFn = func(int) {}
	sleepFn = func(time.Duration) {}

	u := NewUpdater("1.0.0", quietLogger())
	if err := u.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if restarts != 0 {
		t.Errorf("must not touch launchctl when unsupported, restarts=%d", restarts)
	}
	if spawned == 0 {
		t.Error("expected the standalone spawn path")
	}
}

// The daemon applying a dashboard-triggered update carries _ONWATCH_DAEMON=1.
// Inheriting it makes the replacement skip both stopPreviousInstance and
// daemonize, so it loses the port fight and dies while the old binary keeps
// running.
func TestRestartStripsDaemonMarkersFromSpawn(t *testing.T) {
	prevExec, prevExit, prevSleep := execCommand, exitFn, sleepFn
	t.Cleanup(func() { execCommand, exitFn, sleepFn = prevExec, prevExit, prevSleep })

	var got []string
	execCommand = func(name string, args ...string) *exec.Cmd {
		c := prevExec("true")
		return c
	}
	exitFn = func(int) {}
	sleepFn = func(time.Duration) {}

	got = daemonSpawnEnv([]string{"HOME=/Users/x", "_ONWATCH_DAEMON=1", "PATH=/bin", "_ONWATCH_LAUNCHD=1"})
	for _, kv := range got {
		if kv == "_ONWATCH_DAEMON=1" || kv == "_ONWATCH_LAUNCHD=1" {
			t.Errorf("daemonSpawnEnv kept %q", kv)
		}
	}
	if len(got) != 2 {
		t.Errorf("daemonSpawnEnv dropped too much: %v", got)
	}

	// And the spawn path must actually use it.
	u := NewUpdater("1.0.0", quietLogger())
	if err := u.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
}
