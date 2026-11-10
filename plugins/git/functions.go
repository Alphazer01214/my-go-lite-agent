package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type statusHeader struct {
	Branch   string
	Upstream string
	Ahead    int
	Behind   int
	Detached bool
	Unborn   bool
}

type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	if b.limit <= 0 {
		b.buf.Write(data)
		return len(data), nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(data), nil
	}
	if len(data) > remaining {
		b.buf.Write(data[:remaining])
		b.truncated = true
		return len(data), nil
	}
	b.buf.Write(data)
	return len(data), nil
}

func (b *cappedBuffer) String() string {
	return b.buf.String()
}

func gitExecutable() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("LITEAGENT_GIT_PATH")); configured != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured, nil
		}
	}
	if path, err := exec.LookPath("git"); err == nil {
		return path, nil
	}
	if runtime.GOOS == "windows" {
		candidates := []string{}
		if root := strings.TrimSpace(os.Getenv("EXEPATH")); root != "" {
			candidates = append(candidates,
				filepath.Join(root, "git.exe"),
				filepath.Join(root, "..", "cmd", "git.exe"),
				filepath.Join(root, "..", "mingw64", "bin", "git.exe"),
			)
		}
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			if root := strings.TrimSpace(os.Getenv(env)); root != "" {
				candidates = append(candidates, filepath.Join(root, "Git", "cmd", "git.exe"))
			}
		}
		if root := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); root != "" {
			candidates = append(candidates, filepath.Join(root, "Programs", "Git", "cmd", "git.exe"))
		}
		if root := strings.TrimSpace(os.Getenv("USERPROFILE")); root != "" {
			candidates = append(candidates, filepath.Join(root, "scoop", "apps", "git", "current", "cmd", "git.exe"))
		}
		for _, candidate := range candidates {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	return "", pluginsdk.ErrCode(codeGitUnavailable, "git executable not found")
}

func runGit(ctx context.Context, workspace string, timeout time.Duration, limit int, args ...string) (string, bool, error) {
	executable, err := gitExecutable()
	if err != nil {
		return "", false, err
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, executable, args...)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PAGER=cat",
		"PAGER=cat",
		"LC_ALL=C",
		"GIT_OPTIONAL_LOCKS=0",
	)
	stdout := &cappedBuffer{limit: limit}
	stderr := &cappedBuffer{limit: maxGitStderr}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if runCtx.Err() != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return "", stdout.truncated, pluginsdk.ErrCode(pluginsdk.CodeCallDeadlineExceeded, "git command timed out")
		}
		return "", stdout.truncated, pluginsdk.ErrCode(pluginsdk.CodeCallCancelled, "git command cancelled")
	}
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", stdout.truncated, pluginsdk.ErrCode(codeCommandFailed, message)
	}
	return stdout.String(), stdout.truncated, nil
}

func contextError(err error) bool {
	switch pluginsdk.Code(err) {
	case pluginsdk.CodeCallCancelled, pluginsdk.CodeCallDeadlineExceeded:
		return true
	default:
		return false
	}
}

func (p *plugin) resolveWorkspace(ctx context.Context, requested string) (string, error) {
	workspace := strings.TrimSpace(requested)
	if workspace == "" && p.sdk != nil {
		raw, err := p.sdk.Call(ctx, "workspace", "current", json.RawMessage(`{}`))
		if err != nil {
			return "", pluginsdk.ErrCode(codeWorkspaceMissing, "workspace.current: "+err.Error())
		}
		var out struct {
			Workspace *struct {
				Path string `json:"path"`
			} `json:"workspace"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return "", pluginsdk.ErrCode(codeWorkspaceMissing, "bad workspace.current response: "+err.Error())
		}
		if out.Workspace != nil {
			workspace = strings.TrimSpace(out.Workspace.Path)
		}
	}
	if workspace == "" {
		return "", pluginsdk.ErrCode(codeWorkspaceMissing, "workspace is required")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", pluginsdk.ErrCode(codeBadPath, "resolve workspace: "+err.Error())
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", pluginsdk.ErrCode(codeWorkspaceMissing, "workspace is not a directory: "+abs)
	}
	return filepath.Clean(abs), nil
}

func (p *plugin) repoRoot(ctx context.Context, workspace string) (string, bool, error) {
	out, _, err := runGit(ctx, workspace, gitCommandTimeout, 64*1024, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		if contextError(err) || pluginsdk.Code(err) == codeGitUnavailable {
			return "", false, err
		}
		return "", false, nil
	}
	if strings.TrimSpace(out) != "true" {
		return "", false, nil
	}
	rootOut, _, err := runGit(ctx, workspace, gitCommandTimeout, 64*1024, "rev-parse", "--show-toplevel")
	if err != nil {
		if contextError(err) {
			return "", false, err
		}
		return "", false, nil
	}
	root := strings.TrimSpace(rootOut)
	if root == "" {
		return "", false, nil
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return filepath.Clean(root), true, nil
}

func (p *plugin) status(ctx context.Context, requested string) (statusResp, error) {
	workspace, err := p.resolveWorkspace(ctx, requested)
	if err != nil {
		return statusResp{}, err
	}
	resp := statusResp{
		Workspace: workspace,
		Repo:      false,
		Staged:    []fileEntry{},
		Unstaged:  []fileEntry{},
	}
	root, repo, err := p.repoRoot(ctx, workspace)
	if err != nil {
		return statusResp{}, err
	}
	if !repo {
		return resp, nil
	}
	resp.Repo = true
	resp.Root = root

	raw, truncated, err := runGit(ctx, workspace, gitCommandTimeout, maxGitOutput,
		"status", "--porcelain=v1", "--branch", "-uall", "-z")
	if err != nil {
		return statusResp{}, err
	}
	if truncated {
		return statusResp{}, pluginsdk.ErrCode(codeCommandFailed, "git status output exceeded limit")
	}

	prefixOut, _, prefixErr := runGit(ctx, workspace, gitCommandTimeout, 64*1024, "rev-parse", "--show-prefix")
	if prefixErr != nil && contextError(prefixErr) {
		return statusResp{}, prefixErr
	}
	prefix := ""
	if prefixErr == nil {
		prefix = strings.TrimSuffix(strings.TrimSpace(prefixOut), "/")
	}
	header, staged, unstaged, conflicts := parseStatusOutput(raw, prefix)
	resp.Branch = header.Branch
	resp.Upstream = header.Upstream
	resp.Ahead = header.Ahead
	resp.Behind = header.Behind
	resp.Detached = header.Detached
	resp.Unborn = header.Unborn
	resp.Conflicted = conflicts

	stagedStats, _ := p.numstat(ctx, workspace, true)
	unstagedStats, _ := p.numstat(ctx, workspace, false)
	applyStats(staged, stagedStats)
	applyStats(unstaged, unstagedStats)
	sortEntries(staged)
	sortEntries(unstaged)
	resp.Staged = staged
	resp.Unstaged = unstaged
	return resp, nil
}

func scopeStatusPath(path, prefix string) (string, bool) {
	path = filepath.ToSlash(path)
	if prefix == "" {
		return path, true
	}
	prefixWithSlash := prefix + "/"
	if strings.HasPrefix(path, prefixWithSlash) {
		return strings.TrimPrefix(path, prefixWithSlash), true
	}
	return "", false
}

func parseStatusOutput(raw, prefix string) (statusHeader, []fileEntry, []fileEntry, int) {
	tokens := strings.Split(raw, "\x00")
	index := 0
	var header statusHeader
	if len(tokens) > 0 && strings.HasPrefix(tokens[0], "## ") {
		header = parseBranchHeader(tokens[0])
		index = 1
	}
	staged := []fileEntry{}
	unstaged := []fileEntry{}
	conflicts := 0
	for ; index < len(tokens); index++ {
		entry := tokens[index]
		if len(entry) < 4 || entry[2] != ' ' {
			continue
		}
		indexStatus := entry[0]
		worktreeStatus := entry[1]
		path := entry[3:]
		origPath := ""
		if indexStatus == 'R' || indexStatus == 'C' || worktreeStatus == 'R' || worktreeStatus == 'C' {
			if index+1 < len(tokens) {
				index++
				origPath = tokens[index]
			}
		}
		scoped, ok := scopeStatusPath(path, prefix)
		if !ok {
			continue
		}
		scopedOrig := ""
		if origPath != "" {
			if converted, ok := scopeStatusPath(origPath, prefix); ok {
				scopedOrig = converted
			}
		}
		if isConflictPair(indexStatus, worktreeStatus) {
			conflicts++
			file := fileEntry{Path: scoped, OrigPath: scopedOrig, Status: "U", Conflict: true}
			staged = append(staged, file)
			unstaged = append(unstaged, file)
			continue
		}
		if indexStatus != ' ' && indexStatus != '?' {
			staged = append(staged, fileEntry{
				Path:     scoped,
				OrigPath: scopedOrig,
				Status:   string(indexStatus),
			})
		}
		if worktreeStatus != ' ' {
			status := string(worktreeStatus)
			if worktreeStatus == '?' {
				status = "?"
			}
			unstaged = append(unstaged, fileEntry{
				Path:     scoped,
				OrigPath: scopedOrig,
				Status:   status,
			})
		}
	}
	return header, staged, unstaged, conflicts
}

func parseBranchHeader(token string) statusHeader {
	text := strings.TrimPrefix(token, "## ")
	var header statusHeader
	switch {
	case strings.HasPrefix(text, "No commits yet on "):
		header.Unborn = true
		header.Branch = strings.TrimSpace(strings.TrimPrefix(text, "No commits yet on "))
		return header
	case strings.HasPrefix(text, "HEAD (no branch)"):
		header.Detached = true
		return header
	}
	local := text
	if index := strings.Index(text, " ["); index >= 0 {
		local = text[:index]
	}
	if index := strings.Index(local, "..."); index >= 0 {
		header.Branch = local[:index]
		header.Upstream = local[index+3:]
	} else {
		header.Branch = local
	}
	if index := strings.Index(text, "ahead "); index >= 0 {
		header.Ahead = leadingInt(text[index+len("ahead "):])
	}
	if index := strings.Index(text, "behind "); index >= 0 {
		header.Behind = leadingInt(text[index+len("behind "):])
	}
	header.Branch = strings.TrimSpace(header.Branch)
	header.Upstream = strings.TrimSpace(header.Upstream)
	return header
}

func leadingInt(value string) int {
	digits := strings.Builder{}
	for _, char := range value {
		if char < '0' || char > '9' {
			break
		}
		digits.WriteRune(char)
	}
	number, err := strconv.Atoi(digits.String())
	if err != nil {
		return 0
	}
	return number
}

func isConflictPair(indexStatus, worktreeStatus byte) bool {
	switch string([]byte{indexStatus, worktreeStatus}) {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return true
	default:
		return false
	}
}

func (p *plugin) numstat(ctx context.Context, workspace string, staged bool) (map[string]lineStat, error) {
	args := []string{"diff", "--numstat", "-z", "--no-ext-diff", "--relative"}
	if staged {
		args = append(args, "--cached")
	}
	raw, _, err := runGit(ctx, workspace, gitCommandTimeout, maxGitOutput, args...)
	if err != nil {
		return nil, err
	}
	return parseNumstat(raw), nil
}

func parseNumstat(raw string) map[string]lineStat {
	stats := make(map[string]lineStat)
	tokens := strings.Split(raw, "\x00")
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		if token == "" {
			continue
		}
		parts := strings.SplitN(token, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		path := parts[2]
		if path == "" {
			if index+2 < len(tokens) {
				path = tokens[index+2]
				index += 2
			} else {
				continue
			}
		}
		insertions, insertionErr := strconv.Atoi(parts[0])
		deletions, deletionErr := strconv.Atoi(parts[1])
		binary := insertionErr != nil || deletionErr != nil
		stats[path] = lineStat{
			Insertions: insertions,
			Deletions:  deletions,
			Binary:     binary,
		}
	}
	return stats
}

func applyStats(entries []fileEntry, stats map[string]lineStat) {
	for index := range entries {
		stat, ok := stats[entries[index].Path]
		if !ok {
			continue
		}
		entries[index].Insertions = stat.Insertions
		entries[index].Deletions = stat.Deletions
		entries[index].Binary = stat.Binary
	}
}

func sortEntries(entries []fileEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
}

func (p *plugin) branches(ctx context.Context, requested string) (branchesResp, error) {
	workspace, err := p.resolveWorkspace(ctx, requested)
	if err != nil {
		return branchesResp{}, err
	}
	_, repo, err := p.repoRoot(ctx, workspace)
	if err != nil {
		return branchesResp{}, err
	}
	if !repo {
		return branchesResp{}, pluginsdk.ErrCode(codeNotRepo, "workspace is not a git repository")
	}
	raw, truncated, err := runGit(ctx, workspace, gitCommandTimeout, maxGitOutput,
		"for-each-ref",
		"--format=%(refname:short)%00%(HEAD)%00%(upstream:short)%00%(committerdate:unix)%00%(contents:subject)",
		"refs/heads")
	if err != nil {
		return branchesResp{}, err
	}
	if truncated {
		return branchesResp{}, pluginsdk.ErrCode(codeCommandFailed, "git branch output exceeded limit")
	}
	branches := parseBranches(raw)
	current := ""
	for _, branch := range branches {
		if branch.Current {
			current = branch.Name
			break
		}
	}
	detached := false
	if current == "" {
		if out, _, err := runGit(ctx, workspace, gitCommandTimeout, 64*1024, "branch", "--show-current"); err == nil {
			current = strings.TrimSpace(out)
		}
		detached = current == ""
	}
	sort.SliceStable(branches, func(i, j int) bool {
		if branches[i].Current != branches[j].Current {
			return branches[i].Current
		}
		if branches[i].CommittedAt != branches[j].CommittedAt {
			return branches[i].CommittedAt > branches[j].CommittedAt
		}
		return branches[i].Name < branches[j].Name
	})
	return branchesResp{
		Workspace: workspace,
		Current:   current,
		Detached:  detached,
		Branches:  branches,
	}, nil
}

func parseBranches(raw string) []branchInfo {
	branches := []branchInfo{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x00")
		if len(fields) < 4 || strings.TrimSpace(fields[0]) == "" {
			continue
		}
		committedAt, _ := strconv.ParseInt(strings.TrimSpace(fields[3]), 10, 64)
		subject := ""
		if len(fields) > 4 {
			subject = truncateRunes(fields[4], maxSubjectSize)
		}
		branches = append(branches, branchInfo{
			Name:        strings.TrimSpace(fields[0]),
			Current:     strings.TrimSpace(fields[1]) == "*",
			Upstream:    strings.TrimSpace(fields[2]),
			CommittedAt: committedAt,
			Subject:     subject,
		})
	}
	return branches
}

func truncateRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

func (p *plugin) normalizeBranch(ctx context.Context, workspace, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", pluginsdk.ErrCode(codeBadBranch, "branch name is required")
	}
	out, _, err := runGit(ctx, workspace, gitCommandTimeout, 64*1024, "check-ref-format", "--branch", name)
	if err != nil {
		if contextError(err) {
			return "", err
		}
		return "", pluginsdk.ErrCode(codeBadBranch, "invalid branch name: "+name)
	}
	normalized := strings.TrimSpace(out)
	if normalized == "" || strings.HasPrefix(normalized, "-") {
		return "", pluginsdk.ErrCode(codeBadBranch, "invalid branch name: "+name)
	}
	return normalized, nil
}

func (p *plugin) checkout(ctx context.Context, requested, name string) (actionResp, error) {
	workspace, err := p.resolveWorkspace(ctx, requested)
	if err != nil {
		return actionResp{}, err
	}
	_, repo, err := p.repoRoot(ctx, workspace)
	if err != nil {
		return actionResp{}, err
	}
	if !repo {
		return actionResp{}, pluginsdk.ErrCode(codeNotRepo, "workspace is not a git repository")
	}
	branch, err := p.normalizeBranch(ctx, workspace, name)
	if err != nil {
		return actionResp{}, err
	}
	if _, _, err := runGit(ctx, workspace, gitCommandTimeout, 128*1024, "checkout", branch); err != nil {
		return actionResp{}, err
	}
	return actionResp{Workspace: workspace, Branch: branch, OK: true}, nil
}

func (p *plugin) createBranch(ctx context.Context, requested, name string) (actionResp, error) {
	workspace, err := p.resolveWorkspace(ctx, requested)
	if err != nil {
		return actionResp{}, err
	}
	_, repo, err := p.repoRoot(ctx, workspace)
	if err != nil {
		return actionResp{}, err
	}
	if !repo {
		return actionResp{}, pluginsdk.ErrCode(codeNotRepo, "workspace is not a git repository")
	}
	branch, err := p.normalizeBranch(ctx, workspace, name)
	if err != nil {
		return actionResp{}, err
	}
	if _, _, err := runGit(ctx, workspace, gitCommandTimeout, 128*1024, "checkout", "-b", branch); err != nil {
		return actionResp{}, err
	}
	return actionResp{Workspace: workspace, Branch: branch, OK: true}, nil
}

func (p *plugin) deleteBranch(ctx context.Context, requested, name string) (actionResp, error) {
	workspace, err := p.resolveWorkspace(ctx, requested)
	if err != nil {
		return actionResp{}, err
	}
	_, repo, err := p.repoRoot(ctx, workspace)
	if err != nil {
		return actionResp{}, err
	}
	if !repo {
		return actionResp{}, pluginsdk.ErrCode(codeNotRepo, "workspace is not a git repository")
	}
	branch, err := p.normalizeBranch(ctx, workspace, name)
	if err != nil {
		return actionResp{}, err
	}
	current, _, currentErr := runGit(ctx, workspace, gitCommandTimeout, 64*1024, "branch", "--show-current")
	if currentErr == nil && strings.TrimSpace(current) == branch {
		return actionResp{}, pluginsdk.ErrCode(codeBadBranch, "cannot delete the current branch")
	}
	if _, _, err := runGit(ctx, workspace, gitCommandTimeout, 128*1024, "branch", "-d", branch); err != nil {
		return actionResp{}, err
	}
	return actionResp{Workspace: workspace, Branch: branch, OK: true}, nil
}

func cleanRelPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, 0) {
		return "", pluginsdk.ErrCode(codeBadPath, "path is required")
	}
	if filepath.IsAbs(value) || filepath.VolumeName(value) != "" ||
		strings.HasPrefix(value, "/") || strings.HasPrefix(value, "\\") {
		return "", pluginsdk.ErrCode(codeBadPath, "path must stay inside the workspace")
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", pluginsdk.ErrCode(codeBadPath, "path must stay inside the workspace")
	}
	return filepath.ToSlash(cleaned), nil
}

func (p *plugin) diff(ctx context.Context, req diffReq) (diffResp, error) {
	workspace, err := p.resolveWorkspace(ctx, req.Workspace)
	if err != nil {
		return diffResp{}, err
	}
	_, repo, err := p.repoRoot(ctx, workspace)
	if err != nil {
		return diffResp{}, err
	}
	if !repo {
		return diffResp{}, pluginsdk.ErrCode(codeNotRepo, "workspace is not a git repository")
	}
	path, err := cleanRelPath(req.Path)
	if err != nil {
		return diffResp{}, err
	}
	origPath := ""
	if strings.TrimSpace(req.OrigPath) != "" {
		origPath, err = cleanRelPath(req.OrigPath)
		if err != nil {
			return diffResp{}, err
		}
	}
	resp := diffResp{
		Workspace: workspace,
		Path:      path,
		Staged:    req.Staged,
		Untracked: req.Untracked,
	}
	if req.Untracked {
		return resp, nil
	}
	args := []string{"diff", "--no-color", "--no-ext-diff", "--relative"}
	if req.Staged {
		args = append(args, "--cached")
	}
	args = append(args, "--")
	if origPath != "" {
		args = append(args, origPath)
	}
	args = append(args, path)
	raw, truncated, err := runGit(ctx, workspace, gitDiffTimeout, maxDiffOutput, args...)
	if err != nil {
		return diffResp{}, err
	}
	resp.Diff = raw
	resp.Truncated = truncated
	if raw == "" && !req.Staged {
		if _, _, lsErr := runGit(ctx, workspace, time.Second*5, 64*1024,
			"ls-files", "--error-unmatch", "--", path); lsErr != nil && !contextError(lsErr) {
			resp.Untracked = true
		}
	}
	return resp, nil
}
