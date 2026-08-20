package update

import (
	"os"
	"testing"
)

// TestMain neutralises the launchd hooks for the whole package. Without this a
// developer who has the onWatch auto-start agent installed would have their
// real agent kickstarted (and their running onWatch killed) by any test that
// reaches Restart(). Tests that exercise the launchd branch override the hooks
// themselves.
func TestMain(m *testing.M) {
	// GitHub Actions runs jobs under systemd, so INVOCATION_ID is set on the
	// runner and IsSystemd() reports true there but not on a developer
	// machine. Clear it so Restart()'s branch selection is deterministic; the
	// systemd tests set it explicitly.
	os.Unsetenv("INVOCATION_ID")

	launchdSupported = func() bool { return false }
	launchdInstalled = func() bool { return false }
	launchdLoaded = func() bool { return false }
	launchdRestart = func() error { return nil }

	os.Exit(m.Run())
}
