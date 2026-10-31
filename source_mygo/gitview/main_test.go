package gitview

import (
	"os"
	"testing"
)

// TestMain keeps Git from rewriting line endings, so the tests compare the
// same bytes whatever the machine's Git configuration says.
func TestMain(m *testing.M) {
	os.Setenv("GIT_CONFIG_COUNT", "1")
	os.Setenv("GIT_CONFIG_KEY_0", "core.autocrlf")
	os.Setenv("GIT_CONFIG_VALUE_0", "false")
	os.Exit(m.Run())
}
