package discovery

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"

	"github.com/nonchord/kcompass/internal/backend"
)

// TailscaleOptions controls how the Tailscale probe works.
// All fields are optional; zero values select production defaults.
type TailscaleOptions struct {
	// SocketPath is the tailscaled Unix socket. Default: /var/run/tailscale/tailscaled.sock.
	// Used as a fast no-subprocess check before forking tailscale(1).
	SocketPath string
	// RunStatus runs "tailscale status --json". If nil, exec.CommandContext is used.
	// In production this is only attempted when SocketPath exists.
	RunStatus func(ctx context.Context) ([]byte, error)
	// LookupSRV performs the _kcompass._tcp SRV lookup. If nil, net.DefaultResolver is used.
	LookupSRV func(ctx context.Context, service, proto, name string) (string, []*net.SRV, error)
}

// tailscaleStatusJSON is the subset of `tailscale status --json` we need.
type tailscaleStatusJSON struct {
	MagicDNSSuffix string `json:"MagicDNSSuffix"`
}

// TailscaleProbe returns a ProbeFunc that detects Tailscale and performs an
// SRV lookup for _kcompass._tcp.<tailnet-domain>.
func TailscaleProbe(opts TailscaleOptions) ProbeFunc {
	if opts.SocketPath == "" {
		opts.SocketPath = "/var/run/tailscale/tailscaled.sock"
	}
	socketPath := opts.SocketPath

	if opts.RunStatus == nil {
		opts.RunStatus = func(ctx context.Context) ([]byte, error) {
			// Only attempt the subprocess when the socket indicates tailscaled is running.
			if _, err := os.Stat(socketPath); err != nil {
				return nil, err
			}
			return exec.CommandContext(ctx, "tailscale", "status", "--json").Output()
		}
	}
	if opts.LookupSRV == nil {
		opts.LookupSRV = net.DefaultResolver.LookupSRV
	}

	return func(ctx context.Context) (backend.Backend, error) {
		data, err := opts.RunStatus(ctx)
		if err != nil {
			return nil, nil
		}
		var st tailscaleStatusJSON
		if err := json.Unmarshal(data, &st); err != nil || st.MagicDNSSuffix == "" {
			return nil, nil
		}
		return srvToHTTPBackend(ctx, st.MagicDNSSuffix, opts.LookupSRV)
	}
}
