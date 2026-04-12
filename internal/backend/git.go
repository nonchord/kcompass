package backend

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

const lastFetchFile = ".kcompass-last-fetch"

// GitBackend clones or fetches a Git repository and reads cluster YAML files
// from a configurable path within it.
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

// cloneRepo performs an initial clone into cloneDir.
func (b *GitBackend) cloneRepo(ctx context.Context, cloneDir string) error {
	if err := os.MkdirAll(cloneDir, 0o700); err != nil {
		return fmt.Errorf("create clone dir: %w", err)
	}
	auth, err := b.authMethod()
	if err != nil {
		return fmt.Errorf("build auth: %w", err)
	}
	opts := &git.CloneOptions{
		URL:  b.url,
		Auth: auth,
	}
	if b.ref != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(b.ref)
	}
	if _, cloneErr := git.PlainCloneContext(ctx, cloneDir, false, opts); cloneErr != nil {
		_ = os.RemoveAll(cloneDir)
		if isAuthError(cloneErr) {
			// Single %w wrap on the sentinel + %v on the underlying error:
			// errors.Is(err, ErrAccessDenied) still works (so list/connect
			// can catch it and print the friendly message), and the raw
			// form prints cleanly as
			//   "clone <url>: access denied to cluster inventory (<raw>)"
			// instead of the two-phrase colon-chain a double %w produces.
			return fmt.Errorf("clone %s: %w (%v)", b.url, ErrAccessDenied, cloneErr)
		}
		return fmt.Errorf("clone %s: %w", b.url, cloneErr)
	}
	b.writeFetchTimestamp(cloneDir)
	return nil
}

// fetchRepo pulls the latest changes for an existing clone, advancing HEAD.
func (b *GitBackend) fetchRepo(ctx context.Context, cloneDir string) error {
	repo, err := git.PlainOpen(cloneDir)
	if err != nil {
		return fmt.Errorf("open repo: %w", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}
	auth, err := b.authMethod()
	if err != nil {
		return fmt.Errorf("build auth: %w", err)
	}
	opts := &git.PullOptions{Auth: auth}
	if b.ref != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(b.ref)
	}
	err = wt.PullContext(ctx, opts)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		if isAuthError(err) {
			return fmt.Errorf("pull %s: %w (%v)", b.url, ErrAccessDenied, err)
		}
		return fmt.Errorf("pull %s: %w", b.url, err)
	}
	b.writeFetchTimestamp(cloneDir)
	return nil
}

// isAuthError reports whether err indicates an authentication or authorization
// failure that the user should be told about in friendly terms. It matches
// go-git's transport sentinels and, as a fallback, substrings for ssh and
// https auth failures that don't wrap those sentinels.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, transport.ErrAuthenticationRequired) ||
		errors.Is(err, transport.ErrAuthorizationFailed) ||
		errors.Is(err, transport.ErrRepositoryNotFound) {
		return true
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "authentication required"),
		strings.Contains(msg, "authorization failed"),
		strings.Contains(msg, "repository not found"),
		strings.Contains(msg, "Repository not found"),
		strings.Contains(msg, "unable to authenticate"),
		strings.Contains(msg, "permission denied"),
		strings.Contains(msg, "Permission denied"):
		return true
	}
	return false
}

// authMethod selects the appropriate go-git auth from the URL and environment.
func (b *GitBackend) authMethod() (transport.AuthMethod, error) {
	switch {
	case strings.HasPrefix(b.url, "https://") || strings.HasPrefix(b.url, "http://"):
		if token := os.Getenv("GIT_TOKEN"); token != "" {
			return &githttp.BasicAuth{Username: "git", Password: token}, nil
		}
		return nil, nil
	case strings.HasPrefix(b.url, "git@") || strings.HasPrefix(b.url, "ssh://"):
		auth, err := ssh.NewSSHAgentAuth("git")
		if err != nil {
			return ssh.DefaultAuthBuilder("git")
		}
		return auth, nil
	default:
		return nil, nil
	}
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
