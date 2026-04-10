package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/cli"
)

func executeCommand(t *testing.T, args ...string) string {
	t.Helper()
	buf := &bytes.Buffer{}
	root := cli.NewRootCommand()
	root.SetOut(buf)
	root.SetArgs(args)
	require.NoError(t, root.Execute())
	return buf.String()
}

func TestListStub(t *testing.T) {
	assert.Contains(t, executeCommand(t, "list"), "not implemented")
}

func TestConnectStub(t *testing.T) {
	assert.Contains(t, executeCommand(t, "connect", "cluster1"), "not implemented")
}

func TestInitStub(t *testing.T) {
	assert.Contains(t, executeCommand(t, "init", "https://example.com"), "not implemented")
}

func TestBackendsStub(t *testing.T) {
	assert.Contains(t, executeCommand(t, "backends"), "not implemented")
}
