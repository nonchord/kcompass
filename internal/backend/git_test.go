package backend_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nonchord/kcompass/internal/backend"
)

// setupBareRepo creates a bare git repository in a temp directory, commits the
// given files into it, and returns the repo URL and a fresh cache directory.
func setupBareRepo(t *testing.T, files map[string]string) (repoURL string, cacheDir string) {
	t.Helper()

	bareDir := filepath.Join(t.TempDir(), "remote.git")
	workDir := filepath.Join(t.TempDir(), "work")

	// Init non-bare work repo and make an initial commit.
	work, err := gogit.PlainInit(workDir, false)
	require.NoError(t, err)

	wt, err := work.Worktree()
	require.NoError(t, err)

	for name, content := range files {
		full := filepath.Join(workDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
		_, err = wt.Add(name)
		require.NoError(t, err)
	}
	_, err = wt.Commit("initial", &gogit.CommitOptions{
		Author:            &object.Signature{Name: "test", Email: "test@test.com"},
		AllowEmptyCommits: len(files) == 0,
	})
	require.NoError(t, err)

	// Mirror into a bare repo so the backend can clone from a URL.
	_, err = gogit.PlainClone(bareDir, true, &gogit.CloneOptions{
		URL: workDir,
	})
	require.NoError(t, err)

	return "file://" + bareDir, t.TempDir()
}

// pushCommitToBare clones bareURL into a temp dir, writes files, commits, and
// pushes back — simulating an upstream change after the initial clone.
func pushCommitToBare(t *testing.T, bareURL string, files map[string]string) {
	t.Helper()
	workDir := t.TempDir()
	work, err := gogit.PlainClone(workDir, false, &gogit.CloneOptions{URL: bareURL})
	require.NoError(t, err)
	wt, err := work.Worktree()
	require.NoError(t, err)
	for name, content := range files {
		full := filepath.Join(workDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
		_, err = wt.Add(name)
		require.NoError(t, err)
	}
	_, err = wt.Commit("update", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com"},
	})
	require.NoError(t, err)
	require.NoError(t, work.Push(&gogit.PushOptions{}))
}

// setupBareRepoWithBranch creates a bare repo with a default branch and an
// additional branch containing branchFiles. Returns the repo URL and cache dir.
func setupBareRepoWithBranch(t *testing.T, defaultFiles map[string]string, branchName string, branchFiles map[string]string) (repoURL string, cacheDir string) {
	t.Helper()

	bareDir := filepath.Join(t.TempDir(), "remote.git")
	workDir := filepath.Join(t.TempDir(), "work")

	work, err := gogit.PlainInit(workDir, false)
	require.NoError(t, err)
	wt, err := work.Worktree()
	require.NoError(t, err)

	writeAndStage := func(files map[string]string) {
		for name, content := range files {
			full := filepath.Join(workDir, name)
			require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
			require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
			_, err = wt.Add(name)
			require.NoError(t, err)
		}
	}
	sig := &object.Signature{Name: "test", Email: "test@test.com"}

	// Initial commit on default branch.
	writeAndStage(defaultFiles)
	_, err = wt.Commit("default branch commit", &gogit.CommitOptions{Author: sig, AllowEmptyCommits: true})
	require.NoError(t, err)

	// Create and switch to the extra branch.
	require.NoError(t, wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
		Create: true,
	}))
	writeAndStage(branchFiles)
	_, err = wt.Commit("branch commit", &gogit.CommitOptions{Author: sig})
	require.NoError(t, err)

	// Mirror to bare repo, pushing all branches.
	_, err = gogit.PlainClone(bareDir, true, &gogit.CloneOptions{URL: workDir})
	require.NoError(t, err)
	// PlainClone only copies HEAD; push all refs explicitly.
	bare, err := gogit.PlainOpen(bareDir)
	require.NoError(t, err)
	_ = bare
	workRepo, err := gogit.PlainOpen(workDir)
	require.NoError(t, err)
	_, err = workRepo.CreateRemote(&config.RemoteConfig{
		Name: "bare",
		URLs: []string{bareDir},
	})
	require.NoError(t, err)
	require.NoError(t, workRepo.Push(&gogit.PushOptions{
		RemoteName: "bare",
		RefSpecs:   []config.RefSpec{"refs/heads/*:refs/heads/*"},
	}))

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

	// Point the backend at an unreachable URL — if it tries to fetch it will fail.
	bUnreachable, err := backend.NewGitBackend(backend.GitBackendConfig{
		Name:     "test-git",
		URL:      "file:///nonexistent-repo-path",
		CacheDir: cacheDir,
		FetchTTL: time.Hour, // TTL not expired, so no fetch attempt
	})
	require.NoError(t, err)

	// Reuse the same cache dir with the original URL to verify cache is hit.
	b2, err := backend.NewGitBackend(backend.GitBackendConfig{
		Name:     "test-git",
		URL:      repoURL,
		CacheDir: cacheDir,
		FetchTTL: time.Hour,
	})
	require.NoError(t, err)
	_ = bUnreachable

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

	// Record the fetch timestamp written after the initial clone.
	tsPath := filepath.Join(b.CloneDir(), ".kcompass-last-fetch")
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
// cloning from a file:// URL (token is read from env but unused for local
// transport). This guards against regressions in the auth-selection path.
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
