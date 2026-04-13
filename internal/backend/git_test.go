package backend_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/backend"
)

// gitCmd runs a git command in the given directory. If dir is empty, the
// command runs in the process's working directory.
func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), string(out))
}

// setupBareRepo creates a bare git repository in a temp directory, commits the
// given files into it, and returns the repo URL and a fresh cache directory.
func setupBareRepo(t *testing.T, files map[string]string) (repoURL string, cacheDir string) {
	t.Helper()

	workDir := t.TempDir()
	bareDir := filepath.Join(t.TempDir(), "remote.git")

	gitCmd(t, workDir, "init")
	gitCmd(t, workDir, "config", "user.email", "test@test.com")
	gitCmd(t, workDir, "config", "user.name", "test")

	for name, content := range files {
		full := filepath.Join(workDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}

	gitCmd(t, workDir, "add", "-A")
	if len(files) == 0 {
		gitCmd(t, workDir, "commit", "--allow-empty", "-m", "initial")
	} else {
		gitCmd(t, workDir, "commit", "-m", "initial")
	}
	gitCmd(t, workDir, "clone", "--bare", workDir, bareDir)

	return "file://" + bareDir, t.TempDir()
}

// pushCommitToBare clones bareURL into a temp dir, writes files, commits, and
// pushes back — simulating an upstream change after the initial clone.
func pushCommitToBare(t *testing.T, bareURL string, files map[string]string) {
	t.Helper()
	workDir := t.TempDir()
	gitCmd(t, workDir, "clone", bareURL, workDir+"/work")
	work := workDir + "/work"
	gitCmd(t, work, "config", "user.email", "test@test.com")
	gitCmd(t, work, "config", "user.name", "test")
	for name, content := range files {
		full := filepath.Join(work, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	gitCmd(t, work, "add", "-A")
	gitCmd(t, work, "commit", "-m", "update")
	gitCmd(t, work, "push")
}

// setupBareRepoWithBranch creates a bare repo with a default branch and an
// additional branch containing branchFiles. Returns the repo URL and cache dir.
func setupBareRepoWithBranch(t *testing.T, defaultFiles map[string]string, branchName string, branchFiles map[string]string) (repoURL string, cacheDir string) {
	t.Helper()

	workDir := t.TempDir()
	bareDir := filepath.Join(t.TempDir(), "remote.git")

	gitCmd(t, workDir, "init")
	gitCmd(t, workDir, "config", "user.email", "test@test.com")
	gitCmd(t, workDir, "config", "user.name", "test")

	// Initial commit on default branch.
	for name, content := range defaultFiles {
		full := filepath.Join(workDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	gitCmd(t, workDir, "add", "-A")
	gitCmd(t, workDir, "commit", "--allow-empty", "-m", "default branch commit")

	// Create and switch to the extra branch.
	gitCmd(t, workDir, "checkout", "-b", branchName)
	for name, content := range branchFiles {
		full := filepath.Join(workDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	gitCmd(t, workDir, "add", "-A")
	gitCmd(t, workDir, "commit", "-m", "branch commit")

	// Create bare clone (gets HEAD branch only), then push all branches.
	gitCmd(t, workDir, "clone", "--bare", workDir, bareDir)
	gitCmd(t, workDir, "remote", "add", "bare", bareDir)
	gitCmd(t, workDir, "push", "bare", "--all")

	return "file://" + bareDir, t.TempDir()
}

func newGitBackend(t *testing.T, repoURL, cacheDir, repoPath string, fetchTTL time.Duration) *backend.GitBackend {
	t.Helper()
	b, err := backend.NewGitBackend(backend.GitBackendConfig{
		Name:     "test-git",
		URL:      repoURL,
		RepoPath: repoPath,
		CacheDir: cacheDir,
		FetchTTL: fetchTTL,
	})
	require.NoError(t, err)
	return b
}

const singleClusterYAML = `clusters:
  - name: git-cluster1
    description: From git.
    kubeconfig:
      command: [tailscale, configure, kubeconfig, git-cluster1]
`

const secondClusterYAML = `clusters:
  - name: git-cluster2
    description: Second cluster.
    kubeconfig:
      command: [gcloud, container, clusters, get-credentials, git-cluster2, --region=us-east-1]
`

const nonClusterYAML = `kind: SomeOtherThing
data:
  key: value
`

func TestGitBackendList(t *testing.T) {
	repoURL, cacheDir := setupBareRepo(t, map[string]string{
		"clusters.yaml": singleClusterYAML,
	})
	b := newGitBackend(t, repoURL, cacheDir, "", time.Hour)
	records, err := b.List(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "git-cluster1", records[0].Name)
	assert.Equal(t, "From git.", records[0].Description)
}

func TestGitBackendMultipleFiles(t *testing.T) {
	repoURL, cacheDir := setupBareRepo(t, map[string]string{
		"clusters/prod.yaml":    singleClusterYAML,
		"clusters/staging.yaml": secondClusterYAML,
	})
	b := newGitBackend(t, repoURL, cacheDir, "clusters", time.Hour)
	records, err := b.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, records, 2)
	names := []string{records[0].Name, records[1].Name}
	assert.ElementsMatch(t, []string{"git-cluster1", "git-cluster2"}, names)
}

func TestGitBackendSkipsNonClusterYAML(t *testing.T) {
	repoURL, cacheDir := setupBareRepo(t, map[string]string{
		"clusters.yaml": singleClusterYAML,
		"README.yaml":   nonClusterYAML,
	})
	b := newGitBackend(t, repoURL, cacheDir, "", time.Hour)
	records, err := b.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, records, 1)
	assert.Equal(t, "git-cluster1", records[0].Name)
}

func TestGitBackendGet(t *testing.T) {
	repoURL, cacheDir := setupBareRepo(t, map[string]string{
		"clusters.yaml": singleClusterYAML,
	})
	b := newGitBackend(t, repoURL, cacheDir, "", time.Hour)
	rec, err := b.Get(context.Background(), "git-cluster1")
	require.NoError(t, err)
	assert.Equal(t, "git-cluster1", rec.Name)
}

func TestGitBackendGetNotFound(t *testing.T) {
	repoURL, cacheDir := setupBareRepo(t, map[string]string{
		"clusters.yaml": singleClusterYAML,
	})
	b := newGitBackend(t, repoURL, cacheDir, "", time.Hour)
	_, err := b.Get(context.Background(), "nope")
	assert.True(t, errors.Is(err, backend.ErrNotFound))
}

func TestGitBackendCacheHit(t *testing.T) {
	repoURL, cacheDir := setupBareRepo(t, map[string]string{
		"clusters.yaml": singleClusterYAML,
	})
	b := newGitBackend(t, repoURL, cacheDir, "", time.Hour)

	// First call clones the repo.
	_, err := b.List(context.Background())
	require.NoError(t, err)

	// Reuse the same cache dir with the original URL to verify cache is hit.
	b2, err := backend.NewGitBackend(backend.GitBackendConfig{
		Name:     "test-git",
		URL:      repoURL,
		CacheDir: cacheDir,
		FetchTTL: time.Hour,
	})
	require.NoError(t, err)

	records, err := b2.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, records, 1)
}

func TestGitBackendFetchOnExpiry(t *testing.T) {
	files := map[string]string{"clusters.yaml": singleClusterYAML}
	repoURL, cacheDir := setupBareRepo(t, files)

	// Clone with a very short TTL.
	b := newGitBackend(t, repoURL, cacheDir, "", time.Millisecond)
	_, err := b.List(context.Background())
	require.NoError(t, err)

	// Wait for TTL to expire.
	time.Sleep(10 * time.Millisecond)

	// Should attempt a fetch (may be a no-op since nothing changed, but must not error).
	records, err := b.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, records, 1)
}

func TestGitBackendInvalidYAML(t *testing.T) {
	repoURL, cacheDir := setupBareRepo(t, map[string]string{
		"clusters.yaml": "clusters:\n  - !!invalid\n",
	})
	b := newGitBackend(t, repoURL, cacheDir, "", time.Hour)
	_, err := b.List(context.Background())
	require.Error(t, err)
}

func TestGitBackendRepoPathSubdir(t *testing.T) {
	repoURL, cacheDir := setupBareRepo(t, map[string]string{
		"infra/clusters.yaml": singleClusterYAML,
		"other/ignored.yaml":  secondClusterYAML,
	})
	// Only scan infra/ — should not see other/
	b := newGitBackend(t, repoURL, cacheDir, "infra", time.Hour)
	records, err := b.List(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "git-cluster1", records[0].Name)
}

// TestGitBackendFetchFailuresAreNonFatal covers the partial-clone recovery
// path in fetchRepo: if an established clone exists but the remote becomes
// unreachable, List must still succeed with the cached copy rather than
// propagating the fetch error.
func TestGitBackendFetchFailuresAreNonFatal(t *testing.T) {
	repoURL, cacheDir := setupBareRepo(t, map[string]string{
		"clusters.yaml": singleClusterYAML,
	})
	// fetchTTL is very short so the second List triggers a fetch.
	b := newGitBackend(t, repoURL, cacheDir, "", time.Millisecond)

	// First List: clones the bare repo into the cache dir.
	records, err := b.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, records, 1)

	// Rip the remote out from under the backend.
	bareDir := repoURL[len("file://"):]
	require.NoError(t, os.RemoveAll(bareDir))

	// Wait out the tiny TTL so the next List runs a fetch.
	time.Sleep(10 * time.Millisecond)

	// Second List: ensureRepo sees the existing clone, tries to fetch
	// (which fails because the bare repo is gone), and must return the
	// cached records anyway.
	records, err = b.List(context.Background())
	require.NoError(t, err, "fetch failures must not fail List")
	assert.Len(t, records, 1, "cached copy should still be returned")
}

// TestGitBackendFetchFailureLogsWhenVerbose verifies that when a Log callback
// is set (wired to --verbose in production), the swallowed fetch error is
// emitted as a diagnostic rather than silently discarded.
func TestGitBackendFetchFailureLogsWhenVerbose(t *testing.T) {
	repoURL, cacheDir := setupBareRepo(t, map[string]string{
		"clusters.yaml": singleClusterYAML,
	})
	var logged []string
	b, err := backend.NewGitBackend(backend.GitBackendConfig{
		Name:     "test-git",
		URL:      repoURL,
		CacheDir: cacheDir,
		FetchTTL: time.Millisecond,
		Log:      func(s string) { logged = append(logged, s) },
	})
	require.NoError(t, err)

	_, err = b.List(context.Background())
	require.NoError(t, err)

	// Kill the remote.
	require.NoError(t, os.RemoveAll(repoURL[len("file://"):]))
	time.Sleep(10 * time.Millisecond)

	_, err = b.List(context.Background())
	require.NoError(t, err)

	// The Log callback should have received a diagnostic about the failed fetch.
	require.NotEmpty(t, logged, "Log callback must fire on fetch failure")
	assert.Contains(t, logged[0], "fetch from")
	assert.Contains(t, logged[0], "using cached copy")
}

// TestGitBackendFetchPicksUpNewCommits verifies that after the TTL expires a
// subsequent List returns content added in a new upstream commit.
func TestGitBackendFetchPicksUpNewCommits(t *testing.T) {
	repoURL, cacheDir := setupBareRepo(t, map[string]string{
		"clusters.yaml": singleClusterYAML,
	})
	b := newGitBackend(t, repoURL, cacheDir, "", time.Millisecond)

	records, err := b.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, records, 1)

	// Push a second cluster file to the remote.
	pushCommitToBare(t, repoURL, map[string]string{
		"more.yaml": secondClusterYAML,
	})

	// Wait for TTL to expire so the backend fetches on next List.
	time.Sleep(10 * time.Millisecond)

	records, err = b.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, records, 2, "should pick up the new cluster after fetch")
	names := make([]string, len(records))
	for i, r := range records {
		names[i] = r.Name
	}
	assert.ElementsMatch(t, []string{"git-cluster1", "git-cluster2"}, names)
}

// TestGitBackendCacheHitNoFetch verifies that when the TTL has not expired the
// backend reads from the local clone without contacting the remote.
func TestGitBackendCacheHitNoFetch(t *testing.T) {
	repoURL, cacheDir := setupBareRepo(t, map[string]string{
		"clusters.yaml": singleClusterYAML,
	})
	b := newGitBackend(t, repoURL, cacheDir, "", time.Hour)

	_, err := b.List(context.Background())
	require.NoError(t, err)

	tsPath := filepath.Join(singleCloneDir(t, cacheDir), ".kcompass-last-fetch")
	info1, err := os.Stat(tsPath)
	require.NoError(t, err)

	// Push a new commit — if the backend fetches it would update the timestamp.
	pushCommitToBare(t, repoURL, map[string]string{"more.yaml": secondClusterYAML})

	_, err = b.List(context.Background())
	require.NoError(t, err)

	info2, err := os.Stat(tsPath)
	require.NoError(t, err)
	assert.Equal(t, info1.ModTime(), info2.ModTime(), "fetch timestamp must not change within TTL")
}

// singleCloneDir finds the single hash-named subdirectory the git backend
// creates under cacheDir.
func singleCloneDir(t *testing.T, cacheDir string) string {
	t.Helper()
	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "test expects exactly one clone under %s", cacheDir)
	return filepath.Join(cacheDir, entries[0].Name())
}

// TestGitBackendRefCheckout verifies that specifying a Ref checks out that
// branch rather than the default branch.
func TestGitBackendRefCheckout(t *testing.T) {
	defaultYAML := `clusters:
  - name: default-cluster
    description: On default branch.
    kubeconfig:
      command: [echo, default]
`
	branchYAML := `clusters:
  - name: staging-cluster
    description: On staging branch.
    kubeconfig:
      command: [echo, staging]
`
	repoURL, cacheDir := setupBareRepoWithBranch(t,
		map[string]string{"clusters.yaml": defaultYAML},
		"staging",
		map[string]string{"clusters.yaml": branchYAML},
	)

	b, err := backend.NewGitBackend(backend.GitBackendConfig{
		Name:     "test-git",
		URL:      repoURL,
		Ref:      "staging",
		CacheDir: cacheDir,
		FetchTTL: time.Hour,
	})
	require.NoError(t, err)

	records, err := b.List(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "staging-cluster", records[0].Name, "should have checked out the staging branch")
}

// TestGitBackendMissingRepoPath verifies that a repoPath that does not exist
// in the cloned repo returns an error.
func TestGitBackendMissingRepoPath(t *testing.T) {
	repoURL, cacheDir := setupBareRepo(t, map[string]string{
		"clusters.yaml": singleClusterYAML,
	})
	b := newGitBackend(t, repoURL, cacheDir, "nonexistent-subdir", time.Hour)
	_, err := b.List(context.Background())
	require.Error(t, err)
}

// TestGitBackendGitTokenEnvVar verifies that setting GIT_TOKEN does not break
// cloning from a file:// URL. For file:// URLs, the token is not embedded
// since there's no host to authenticate to.
func TestGitBackendGitTokenEnvVar(t *testing.T) {
	repoURL, cacheDir := setupBareRepo(t, map[string]string{
		"clusters.yaml": singleClusterYAML,
	})
	t.Setenv("GIT_TOKEN", "test-token-value")
	b := newGitBackend(t, repoURL, cacheDir, "", time.Hour)
	records, err := b.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, records, 1)
}

func TestGitBackendContextCancelled(t *testing.T) {
	// Use a URL that will never respond to trigger the context cancellation.
	b, err := backend.NewGitBackend(backend.GitBackendConfig{
		Name:     "test-git",
		URL:      "https://192.0.2.1/nonexistent.git", // TEST-NET, guaranteed unreachable
		CacheDir: t.TempDir(),
		FetchTTL: 0,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = b.List(ctx)
	require.Error(t, err)
}
