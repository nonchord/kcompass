package backend

import "strings"

// NewBackendFromURL creates a Backend from a URL string, inferring the type.
// SSH and HTTPS/HTTP git URLs produce a GitBackend; plain paths produce a LocalBackend.
// This is the single authoritative place where URL → backend type inference lives,
// shared by the CLI init command and the discovery probes.
func NewBackendFromURL(rawURL string) (Backend, error) {
	if isGitURL(rawURL) {
		return NewNamedBackendFromURL("git:"+rawURL, rawURL)
	}
	return NewNamedBackendFromURL("local:"+rawURL, rawURL)
}

// NewNamedBackendFromURL creates a Backend with an explicit name, inferring the
// backend type from rawURL. The name is typically "source:url" where source
// encodes discovery provenance (e.g. "tailscale:git@github.com:company/clusters").
func NewNamedBackendFromURL(name, rawURL string) (Backend, error) {
	if isGitURL(rawURL) {
		return NewGitBackend(GitBackendConfig{Name: name, URL: rawURL})
	}
	return NewLocalBackend(name, rawURL)
}

func isGitURL(rawURL string) bool {
	switch {
	case strings.HasPrefix(rawURL, "https://"),
		strings.HasPrefix(rawURL, "http://"),
		strings.HasPrefix(rawURL, "git@"),
		strings.HasPrefix(rawURL, "git://"),
		strings.HasPrefix(rawURL, "ssh://"):
		return true
	}
	return false
}
