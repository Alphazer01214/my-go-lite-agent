package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := gitExecutable(); err != nil {
		t.Skip("git is not installed")
	}
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	executable, err := gitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"LC_ALL=C",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTestRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "-c", "init.defaultBranch=main", "init", "-q")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	gitRun(t, dir, "config", "user.name", "Test")
	gitRun(t, dir, "config", "commit.gpgsign", "false")
	gitRun(t, dir, "config", "core.autocrlf", "false")
	writeTestFile(t, dir, "a.txt", "one\ntwo\n")
	gitRun(t, dir, "add", "a.txt")
	gitRun(t, dir, "commit", "-q", "--no-verify", "-m", "init")
	return dir
}

func findEntry(entries []fileEntry, path string) *fileEntry {
	for index := range entries {
		if entries[index].Path == path {
			return &entries[index]
		}
	}
	return nil
}

func TestStatusParsesWorktreeAndIndex(t *testing.T) {
	repo := newTestRepo(t)
	p := &plugin{}

	writeTestFile(t, repo, "a.txt", "one\nTWO\nthree\n")
	writeTestFile(t, repo, "dir/untracked.txt", "new\n")
	writeTestFile(t, repo, "b.txt", "added\n")
	gitRun(t, repo, "add", "b.txt")
	writeTestFile(t, repo, "b.txt", "added\nmore\n")

	resp, err := p.status(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Repo || resp.Branch != "main" || resp.Detached {
		t.Fatalf("repo state = %+v", resp)
	}
	if resp.Conflicted != 0 {
		t.Fatalf("conflicted = %d", resp.Conflicted)
	}

	modified := findEntry(resp.Unstaged, "a.txt")
	if modified == nil || modified.Status != "M" {
		t.Fatalf("a.txt unstaged = %+v", modified)
	}
	if modified.Insertions == 0 && modified.Deletions == 0 {
		t.Fatalf("a.txt stats = %+v", modified)
	}
	untracked := findEntry(resp.Unstaged, "dir/untracked.txt")
	if untracked == nil || untracked.Status != "?" {
		t.Fatalf("untracked = %+v", untracked)
	}
	added := findEntry(resp.Staged, "b.txt")
	if added == nil || added.Status != "A" {
		t.Fatalf("b.txt staged = %+v", added)
	}
	both := findEntry(resp.Unstaged, "b.txt")
	if both == nil || both.Status != "M" {
		t.Fatalf("b.txt unstaged = %+v", both)
	}
}

func TestStatusScopesWorkspaceSubdirectory(t *testing.T) {
	repo := newTestRepo(t)
	writeTestFile(t, repo, "inside.txt", "inside\n")
	gitRun(t, repo, "add", "inside.txt")
	gitRun(t, repo, "commit", "-q", "--no-verify", "-m", "inside")
	sub := filepath.Join(repo, "sub")
	writeTestFile(t, repo, "sub/in.txt", "in\n")
	gitRun(t, repo, "add", "sub/in.txt")
	gitRun(t, repo, "commit", "-q", "--no-verify", "-m", "sub")
	writeTestFile(t, repo, "outside.txt", "outside change\n")
	writeTestFile(t, repo, "sub/in.txt", "in changed\n")

	p := &plugin{}
	resp, err := p.status(context.Background(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Repo || resp.Branch != "main" {
		t.Fatalf("repo = %+v", resp)
	}
	if findEntry(resp.Unstaged, "outside.txt") != nil {
		t.Fatalf("status must be scoped to the workspace subdirectory; root=%q workspace=%q entries=%+v", resp.Root, sub, resp.Unstaged)
	}
	inside := findEntry(resp.Unstaged, "in.txt")
	if inside == nil || inside.Status != "M" {
		t.Fatalf("inside = %+v, all = %+v", inside, resp.Unstaged)
	}
	diff, err := p.diff(context.Background(), diffReq{Workspace: sub, Path: "in.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff.Diff, "+in changed") {
		t.Fatalf("subdir diff = %q", diff.Diff)
	}
}

func TestUnbornRepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "-c", "init.defaultBranch=main", "init", "-q")
	writeTestFile(t, dir, "first.txt", "first\n")
	gitRun(t, dir, "add", "first.txt")

	p := &plugin{}
	status, err := p.status(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Repo || !status.Unborn || status.Branch != "main" {
		t.Fatalf("unborn status = %+v", status)
	}
	added := findEntry(status.Staged, "first.txt")
	if added == nil || added.Status != "A" {
		t.Fatalf("staged = %+v", status.Staged)
	}
	branches, err := p.branches(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(branches.Branches) != 0 {
		t.Fatalf("unborn branches = %+v", branches)
	}
}

func TestBranchesAndActions(t *testing.T) {
	repo := newTestRepo(t)
	p := &plugin{}
	ctx := context.Background()

	initial, err := p.branches(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Current != "main" || len(initial.Branches) != 1 || !initial.Branches[0].Current {
		t.Fatalf("initial branches = %+v", initial)
	}
	if initial.Branches[0].Subject != "init" {
		t.Fatalf("branch subject = %q", initial.Branches[0].Subject)
	}

	created, err := p.createBranch(ctx, repo, "feature/x")
	if err != nil {
		t.Fatal(err)
	}
	if !created.OK || created.Branch != "feature/x" {
		t.Fatalf("created = %+v", created)
	}
	current, err := p.branches(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if current.Current != "feature/x" {
		t.Fatalf("current = %q", current.Current)
	}

	if _, err := p.checkout(ctx, repo, "main"); err != nil {
		t.Fatal(err)
	}
	back, err := p.branches(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if back.Current != "main" {
		t.Fatalf("current after checkout = %q", back.Current)
	}

	if _, err := p.deleteBranch(ctx, repo, "feature/x"); err != nil {
		t.Fatal(err)
	}
	afterDelete, err := p.branches(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, branch := range afterDelete.Branches {
		if branch.Name == "feature/x" {
			t.Fatal("feature/x was not deleted")
		}
	}

	if _, err := p.deleteBranch(ctx, repo, "main"); pluginsdk.Code(err) != codeBadBranch {
		t.Fatalf("delete current branch err = %v", err)
	}
	if _, err := p.createBranch(ctx, repo, "bad name"); pluginsdk.Code(err) != codeBadBranch {
		t.Fatalf("create invalid branch err = %v", err)
	}
}

func TestDiffUnstagedStagedAndUntracked(t *testing.T) {
	repo := newTestRepo(t)
	p := &plugin{}
	ctx := context.Background()
	writeTestFile(t, repo, "a.txt", "one\nTWO\n")

	unstaged, err := p.diff(ctx, diffReq{Workspace: repo, Path: "a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(unstaged.Diff, "-two") || !strings.Contains(unstaged.Diff, "+TWO") {
		t.Fatalf("unstaged diff = %q", unstaged.Diff)
	}

	gitRun(t, repo, "add", "a.txt")
	staged, err := p.diff(ctx, diffReq{Workspace: repo, Path: "a.txt", Staged: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(staged.Diff, "+TWO") {
		t.Fatalf("staged diff = %q", staged.Diff)
	}

	writeTestFile(t, repo, "u.txt", "untracked\n")
	untracked, err := p.diff(ctx, diffReq{Workspace: repo, Path: "u.txt", Untracked: true})
	if err != nil {
		t.Fatal(err)
	}
	if !untracked.Untracked || untracked.Diff != "" {
		t.Fatalf("untracked = %+v", untracked)
	}

	if _, err := p.diff(ctx, diffReq{Workspace: repo, Path: "../outside.txt"}); pluginsdk.Code(err) != codeBadPath {
		t.Fatalf("traversal err = %v", err)
	}
}

func TestStatusNotRepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	p := &plugin{}
	resp, err := p.status(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Repo {
		t.Fatal("temp dir must not be a repository")
	}
	if _, err := p.branches(context.Background(), dir); pluginsdk.Code(err) != codeNotRepo {
		t.Fatalf("branches err = %v", err)
	}
}

func TestMissingWorkspace(t *testing.T) {
	p := &plugin{}
	if _, err := p.status(context.Background(), ""); pluginsdk.Code(err) != codeWorkspaceMissing {
		t.Fatalf("status err = %v", err)
	}
}
