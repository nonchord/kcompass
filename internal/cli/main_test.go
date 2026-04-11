package cli_test

import (
	"os"
	"testing"
)

// TestMain redirects os.Stdin to /dev/null so that tests behave the same
// whether `go test` is invoked from a terminal or from CI. Without this, the
// operator add command's interactive path fires when stdin is a TTY and
// breaks tests that intentionally omit required flags to exercise error paths.
func TestMain(m *testing.M) {
	if devnull, err := os.Open(os.DevNull); err == nil {
		os.Stdin = devnull
	}
	os.Exit(m.Run())
}
