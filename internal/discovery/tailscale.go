package discovery

import (
	"context"
	"encoding/json"
	"fmt"
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
	// LookupTXT performs the kcompass.<tailnet> TXT lookup. If nil, net.DefaultResolver is used.
	LookupTXT func(ctx context.Context, name string) ([]string, error)
	// Log, when non-nil, receives diagnostic messages during probing.
	Log func(string)
}

// tailscaleStatusJSON is the subset of `tailscale status --json` we need.
type tailscaleStatusJSON struct {
	MagicDNSSuffix string `json:"MagicDNSSuffix"`
}

// TailscaleProbe returns a ProbeFunc that detects Tailscale, determines the
// tailnet's MagicDNS suffix, and looks up a "kcompass.<suffix>" TXT record.
func TailscaleProbe(opts TailscaleOptions) ProbeFunc {
	if opts.SocketPath == "" {
		opts.SocketPath = "/var/run/tailscale/tailscaled.sock"
	}
	socketPath := opts.SocketPath

	if opts.RunStatus == nil {
		opts.RunStatus = func(ctx context.Context) ([]byte, error) {
			if _, err := os.Stat(socketPath); err != nil {
				return nil, err
			}
			return exec.CommandContext(ctx, "tailscale", "status", "--json").Output()
		}
	}
	if opts.LookupTXT == nil {
		opts.LookupTXT = net.DefaultResolver.LookupTXT
	}

	return func(ctx context.Context) (backend.Backend, error) {
		data, err := opts.RunStatus(ctx)
		if err != nil {
			if opts.Log != nil {
				opts.Log(fmt.Sprintf("discovery: tailscale: not detected (%v)", err))
			}
			return nil, nil
		}
		var st tailscaleStatusJSON
		if err := json.Unmarshal(data, &st); err != nil || st.MagicDNSSuffix == "" {
			if opts.Log != nil {
				opts.Log("discovery: tailscale: no MagicDNS suffix in status output")
			}
			return nil, nil
		}
		if opts.Log != nil {
			opts.Log(fmt.Sprintf("discovery: tailscale: MagicDNS suffix = %s", st.MagicDNSSuffix))
		}
		return txtBackend(ctx, "tailscale", st.MagicDNSSuffix, opts.LookupTXT, opts.Log)
	}
}
