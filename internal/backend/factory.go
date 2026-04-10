package backend

import "strings"

// NewBackendFromURL creates a Backend from a URL string, inferring the type.
// SSH and HTTPS/HTTP git URLs produce a GitBackend; plain paths produce a LocalBackend.
// This is the single authoritative place where URL → backend type inference lives,
// shared by the CLI init command and the discovery probes.
func NewBackendFromURL(rawURL string) (Backend, error) {
	switch {
	case strings.HasPrefix(rawURL, "https://"),
		strings.HasPrefix(rawURL, "http://"),
		strings.HasPrefix(rawURL, "git@"),
		strings.HasPrefix(rawURL, "git://"),
		strings.HasPrefix(rawURL, "ssh://"):
		return NewGitBackend(GitBackendConfig{
			Name: "git:" + rawURL,
			URL:  rawURL,
		})
	default:
		return NewLocalBackend("local:"+rawURL, rawURL)
	}
}
