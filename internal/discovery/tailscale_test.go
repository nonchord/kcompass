package discovery_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/discovery"
)

func tailscaleStatus(suffix string) func(ctx context.Context) ([]byte, error) {
	return func(_ context.Context) ([]byte, error) {
		return []byte(`{"MagicDNSSuffix":"` + suffix + `"}`), nil
	}
}

func tailscaleStatusErr(err error) func(ctx context.Context) ([]byte, error) {
	return func(_ context.Context) ([]byte, error) { return nil, err }
}

func TestTailscaleProbeFound(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: tailscaleStatus("tailnet.ts.net"),
		LookupTXT: mockTXT(map[string][]string{
			"kcompass.tailnet.ts.net": {"v=kc1; backend=git@github.com:company/clusters"},
		}),
	})
	b, err := probe(context.Background())
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Contains(t, b.Name(), "github.com")
}

func TestTailscaleProbeStatusFails(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: tailscaleStatusErr(errors.New("not installed")),
		LookupTXT: mockTXT(map[string][]string{}),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestTailscaleProbeNoTXTRecord(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: tailscaleStatus("tailnet.ts.net"),
		LookupTXT: mockTXT(map[string][]string{}),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestTailscaleProbeInvalidJSON(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: func(_ context.Context) ([]byte, error) { return []byte("not json"), nil },
		LookupTXT: mockTXT(map[string][]string{}),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestTailscaleProbeEmptyDNSSuffix(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: func(_ context.Context) ([]byte, error) {
			return []byte(`{"MagicDNSSuffix":""}`), nil
		},
		LookupTXT: mockTXT(map[string][]string{}),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestTailscaleProbeContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: func(ctx context.Context) ([]byte, error) {
			return nil, ctx.Err()
		},
		LookupTXT: mockTXT(map[string][]string{}),
	})
	b, err := probe(ctx)
	assert.NoError(t, err)
	assert.Nil(t, b)
}

// TestTailscaleProbeDefaultSocketAbsent exercises the default RunStatus path
// where the socket is absent — it should return nil without forking a subprocess.
func TestTailscaleProbeDefaultSocketAbsent(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		SocketPath: filepath.Join(t.TempDir(), "nonexistent.sock"),
		LookupTXT:  mockTXT(map[string][]string{}),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestTailscaleProbeSocketPath(t *testing.T) {
	socketFile := filepath.Join(t.TempDir(), "tailscaled.sock")
	require.NoError(t, os.WriteFile(socketFile, nil, 0o600))

	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		SocketPath: socketFile,
		RunStatus:  tailscaleStatus("example.ts.net"),
		LookupTXT: mockTXT(map[string][]string{
			"kcompass.example.ts.net": {"v=kc1; backend=git@github.com:org/clusters"},
		}),
	})
	b, err := probe(context.Background())
	require.NoError(t, err)
	require.NotNil(t, b)
}

func TestTailscaleProbeTXTError(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: tailscaleStatus("tailnet.ts.net"),
		LookupTXT: func(_ context.Context, _ string) ([]string, error) {
			return nil, errors.New("NXDOMAIN")
		},
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestTailscaleProbeWrongTXTFormat(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: tailscaleStatus("tailnet.ts.net"),
		LookupTXT: mockTXT(map[string][]string{
			"kcompass.tailnet.ts.net": {"v=spf1 include:example.com ~all"},
		}),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}
