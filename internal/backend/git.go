package backend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const lastFetchFile = ".kcompass-last-fetch"

// GitBackend clones or fetches a Git repository and reads cluster YAML files
// from a configurable path within it. It shells out to the system git binary,
// inheriting the user's SSH config, credential helpers, and other git
// configuration.
type GitBackend struct {
	name     string
	url      string
	repoPath string
	ref      string
	cacheDir string
	fetchTTL time.Duration
	log      func(string)
}

// GitBackendConfig holds all options for NewGitBackend.
type GitBackendConfig struct {
	Name     string
	URL      string
	RepoPath string        // subdirectory within the repo to scan; "" means repo root
	Ref      string        // branch/tag/SHA; "" means the default branch
	CacheDir string        // root cache dir; "" defaults to ~/.kcompass/cache/git
	FetchTTL time.Duration // how often to fetch from remote; 0 means always fetch
	// Log, when non-nil, receives diagnostic messages for operations that
	// are silently best-effort (fetch failures on an existing clone, cache
	// timestamp write errors). Wired to --verbose in the CLI.
	Log func(string)
}

// NewGitBackend creates a GitBackend from the provided config.
func NewGitBackend(cfg GitBackendConfig) (*GitBackend, error) {
	cacheDir := cfg.CacheDir
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("git backend: resolve home dir: %w", err)
		}
		cacheDir = filepath.Join(home, ".kcompass", "cache", "git")
	} else {
		expanded, err := expandPath(cacheDir)
		if err != nil {
			return nil, fmt.Errorf("git backend: expand cache dir: %w", err)
		}
		cacheDir = expanded
	}
	return &GitBackend{
		name:     cfg.Name,
		url:      cfg.URL,
		repoPath: cfg.RepoPath,
		ref:      cfg.Ref,
		cacheDir: cacheDir,
		fetchTTL: cfg.FetchTTL,
		log:      cfg.Log,
	}, nil
}

// Name implements Backend.
func (b *GitBackend) Name() string { return b.name }

// List implements Backend.
func (b *GitBackend) List(ctx context.Context) ([]ClusterRecord, error) {
	cloneDir := b.cloneDir()
	if err := b.ensureRepo(ctx, cloneDir); err != nil {
		return nil, fmt.Errorf("git backend: %w", err)
	}
	scanPath := cloneDir
	if b.repoPath != "" {
		scanPath = filepath.Join(cloneDir, b.repoPath)
	}
	records, err := scanClusterFiles(scanPath)
	if err != nil {
		return nil, fmt.Errorf("git backend: scan %s: %w", scanPath, err)
	}
	return records, nil
}

// Get implements Backend.
func (b *GitBackend) Get(ctx context.Context, name string) (*ClusterRecord, error) {
	records, err := b.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range records {
		if records[i].Name == name {
			return &records[i], nil
		}
	}
	return nil, ErrNotFound
}

// cloneDir returns the local directory used for this repository's clone.
// The path is a hash-named subdirectory under b.cacheDir, so two backends
// pointed at different URLs never collide even when they share a cache
// root.
func (b *GitBackend) cloneDir() string {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(b.url)))
	return filepath.Join(b.cacheDir, hash)
}

// ensureRepo clones the repo if absent, or fetches if the TTL has expired.
func (b *GitBackend) ensureRepo(ctx context.Context, cloneDir string) error {
	if _, err := os.Stat(cloneDir); os.IsNotExist(err) {
		return b.cloneRepo(ctx, cloneDir)
	}
	if b.fetchTTL == 0 || b.fetchExpired(cloneDir) {
		// Fetch failures are non-fatal: work with the cached copy.
		// Log the error when --verbose is set so operators debugging
		// stale-cache issues have a signal.
		if err := b.fetchRepo(ctx, cloneDir); err != nil && b.log != nil {
			b.log(fmt.Sprintf("git backend: fetch from %s failed, using cached copy: %v", b.url, err))
		}
	}
	return nil
}

// fetchExpired reports whether the last-fetch timestamp is older than fetchTTL.
func (b *GitBackend) fetchExpired(cloneDir string) bool {
	data, err := os.ReadFile(filepath.Join(cloneDir, lastFetchFile))
	if err != nil {
		return true
	}
	var ts time.Time
	if err := ts.UnmarshalText(data); err != nil {
		return true
	}
	return time.Since(ts) > b.fetchTTL
}

// writeFetchTimestamp records the current time so fetchExpired can check it.
// Errors are logged (if a logger is set) but not returned — a failed
// timestamp write means the next List re-fetches unnecessarily, which is
// extra work but not incorrect.
func (b *GitBackend) writeFetchTimestamp(cloneDir string) {
	data, err := time.Now().MarshalText()
	if err != nil {
		return
	}
	if err := os.WriteFile(filepath.Join(cloneDir, lastFetchFile), data, 0o600); err != nil {
		if b.log != nil {
			b.log(fmt.Sprintf("git backend: write fetch timestamp: %v", err))
		}
	}
}

// effectiveURL returns the clone URL, embedding GIT_TOKEN credentials for
// HTTPS URLs. The token is stored in the clone's remote config, which is
// acceptable because the clone lives in kcompass's private cache directory.
func (b *GitBackend) effectiveURL() string {
	if token := os.Getenv("GIT_TOKEN"); token != "" {
		if after, ok := strings.CutPrefix(b.url, "https://"); ok {
			return "https://git:" + token + "@" + after
		}
		if after, ok := strings.CutPrefix(b.url, "http://"); ok {
			return "http://git:" + token + "@" + after
		}
	}
	return b.url
}

// cloneRepo performs an initial clone into cloneDir.
func (b *GitBackend) cloneRepo(ctx context.Context, cloneDir string) error {
	if err := os.MkdirAll(filepath.Dir(cloneDir), 0o700); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	args := []string{"clone", "--single-branch"}
	if b.ref != "" {
		args = append(args, "--branch", b.ref)
	}
	args = append(args, b.effectiveURL(), cloneDir)

	stderr, err := b.runGit(ctx, "", args...)
	if err != nil {
		_ = os.RemoveAll(cloneDir)
		if isGitAuthError(stderr) {
			return fmt.Errorf("clone %s: %w (%s)", b.url, ErrAccessDenied, firstLine(stderr))
		}
		return fmt.Errorf("clone %s: %s", b.url, firstLine(stderr))
	}
	b.writeFetchTimestamp(cloneDir)
	return nil
}

// fetchRepo pulls the latest changes for an existing clone, advancing HEAD.
func (b *GitBackend) fetchRepo(ctx context.Context, cloneDir string) error {
	// Update the remote URL so credential changes (GIT_TOKEN) take effect.
	if _, err := b.runGit(ctx, cloneDir, "remote", "set-url", "origin", b.effectiveURL()); err != nil {
		return fmt.Errorf("set remote url: %w", err)
	}

	stderr, err := b.runGit(ctx, cloneDir, "pull", "--ff-only")
	if err != nil {
		if isGitAuthError(stderr) {
			return fmt.Errorf("pull %s: %w (%s)", b.url, ErrAccessDenied, firstLine(stderr))
		}
		return fmt.Errorf("pull %s: %s", b.url, firstLine(stderr))
	}
	b.writeFetchTimestamp(cloneDir)
	return nil
}

// runGit executes a git command and returns its stderr output.
// GIT_TERMINAL_PROMPT=0 is set to prevent interactive credential prompts
// that would hang the process.
func (b *GitBackend) runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // args are constructed internally, not from user input
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimSpace(stderr.String()), err
}

// isGitAuthError reports whether git's stderr output indicates an
// authentication or authorization failure.
func isGitAuthError(stderr string) bool {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "authentication required"),
		strings.Contains(lower, "authorization failed"),
		strings.Contains(lower, "repository not found"),
		strings.Contains(lower, "unable to authenticate"),
		strings.Contains(lower, "permission denied"),
		strings.Contains(lower, "could not read from remote repository"),
		strings.Contains(lower, "could not read username"),
		strings.Contains(lower, "invalid credentials"),
		strings.Contains(lower, "terminal prompts disabled"):
		return true
	}
	return false
}

// firstLine returns the first non-empty line of s, or s itself if single-line.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// scanClusterFiles walks dir and parses every .yaml file that has a top-level
// "clusters" key. Files without that key are silently skipped.
func scanClusterFiles(dir string) ([]ClusterRecord, error) {
	var records []ClusterRecord
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		recs, parseErr := readClusterFile(path)
		if parseErr != nil {
			return parseErr
		}
		records = append(records, recs...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}
