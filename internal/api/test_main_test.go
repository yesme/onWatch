package api

import (
	"os"
	"testing"
)

// TestMain runs before all tests in the api package. It enables test mode
// to prevent tests from reading or writing real credentials in the macOS
// Keychain or Linux keyring. Without this, tests that call
// WriteAnthropicCredentials or DetectAnthropicToken can overwrite the user's
// real Claude Code OAuth tokens, causing Claude Code to be logged out.
//
// It also clears OPENCODE_HOME/XDG_DATA_HOME so package-level detection does
// not inherit a developer's override. Individual tests that must stay hermetic
// should call isolateOpenCodeEnv (pins both to empty temp dirs) because
// clearing alone can still fall through to the real UserHomeDir path.
func TestMain(m *testing.M) {
	SetTestMode(true)
	os.Unsetenv("OPENCODE_HOME")
	os.Unsetenv("XDG_DATA_HOME")
	os.Exit(m.Run())
}
