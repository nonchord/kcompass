package discovery_test

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/discovery"
)

func mockSRV(target string, port uint16) func(ctx context.Context, service, proto, name string) (string, []*net.SRV, error) {
	return func(_ context.Context, _, _, _ string) (string, []*net.SRV, error) {
		return "", []*net.SRV{{Target: target, Port: port, Priority: 0, Weight: 0}}, nil
	}
}

func mockSRVErr(err error) func(ctx context.Context, service, proto, name string) (string, []*net.SRV, error) {
	return func(_ context.Context, _, _, _ string) (string, []*net.SRV, error) {
		return "", nil, err
	}
}

func mockSRVEmpty() func(ctx context.Context, service, proto, name string) (string, []*net.SRV, error) {
	return func(_ context.Context, _, _, _ string) (string, []*net.SRV, error) {
		return "", nil, nil
	}
}

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
		LookupSRV: mockSRV("kcompass.tailnet.ts.net.", 8443),
	})
	b, err := probe(context.Background())
	require.NoError(t, err)
	require.NotNil(t, b)
	assert.Contains(t, b.Name(), "tailnet.ts.net")
}

func TestTailscaleProbeStatusFails(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: tailscaleStatusErr(errors.New("not installed")),
		LookupSRV: mockSRV("irrelevant.", 8443),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestTailscaleProbeNoSRVRecord(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: tailscaleStatus("tailnet.ts.net"),
		LookupSRV: mockSRVEmpty(),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestTailscaleProbeInvalidJSON(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: func(_ context.Context) ([]byte, error) { return []byte("not json"), nil },
		LookupSRV: mockSRV("irrelevant.", 8443),
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
		LookupSRV: mockSRV("irrelevant.", 8443),
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
		LookupSRV: mockSRV("irrelevant.", 8443),
	})
	b, err := probe(ctx)
	assert.NoError(t, err) // errors are swallowed
	assert.Nil(t, b)
}

func TestTailscaleProbeSocketPath(t *testing.T) {
	// Verify SocketPath is used by the default RunStatus: when socket is absent,
	// production would skip the subprocess. Here we inject RunStatus directly
	// so just check the production path accepts an explicit SocketPath.
	socketFile := filepath.Join(t.TempDir(), "tailscaled.sock")
	require.NoError(t, os.WriteFile(socketFile, nil, 0o600))

	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		SocketPath: socketFile,
		RunStatus:  tailscaleStatus("example.ts.net"),
		LookupSRV:  mockSRV("kcompass.example.ts.net.", 443),
	})
	b, err := probe(context.Background())
	require.NoError(t, err)
	require.NotNil(t, b)
}

func TestTailscaleProbeSRVError(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: tailscaleStatus("tailnet.ts.net"),
		LookupSRV: mockSRVErr(errors.New("NXDOMAIN")),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

// TestTailscaleProbeDefaultSocketAbsent exercises the default RunStatus path
// where the socket is absent — it should return nil without forking a subprocess.
func TestTailscaleProbeDefaultSocketAbsent(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		SocketPath: filepath.Join(t.TempDir(), "nonexistent.sock"),
		// RunStatus is intentionally nil to use the default implementation.
		LookupSRV: mockSRV("irrelevant.", 8443),
	})
	b, err := probe(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestTailscaleProbeURLFormat(t *testing.T) {
	probe := discovery.TailscaleProbe(discovery.TailscaleOptions{
		RunStatus: tailscaleStatus("mynet.ts.net"),
		LookupSRV: mockSRV("server.mynet.ts.net.", 8080),
	})
	b, err := probe(context.Background())
	require.NoError(t, err)
	require.NotNil(t, b)
	// Backend is an HTTPBackend; its name encodes the domain.
	assert.Contains(t, b.Name(), "mynet.ts.net")
}
