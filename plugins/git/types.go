package main

import "time"

const (
	gitCommandTimeout = 15 * time.Second
	gitDiffTimeout    = 20 * time.Second

	maxGitOutput   = 4 << 20
	maxDiffOutput  = 512 * 1024
	maxGitStderr   = 32 * 1024
	maxSubjectSize = 240

	codeWorkspaceMissing = "git_workspace_missing"
	codeNotRepo          = "git_not_repo"
	codeGitUnavailable   = "git_unavailable"
	codeCommandFailed    = "git_command_failed"
	codeBadPath          = "git_bad_path"
	codeBadBranch        = "git_bad_branch"
)

type statusReq struct {
	Workspace string `json:"workspace,omitempty"`
}

type branchesReq struct {
	Workspace string `json:"workspace,omitempty"`
}

type diffReq struct {
	Workspace string `json:"workspace,omitempty"`
	Path      string `json:"path"`
	OrigPath  string `json:"orig_path,omitempty"`
	Staged    bool   `json:"staged,omitempty"`
	Untracked bool   `json:"untracked,omitempty"`
}

type checkoutReq struct {
	Workspace string `json:"workspace,omitempty"`
	Branch    string `json:"branch"`
}

type branchCreateReq struct {
	Workspace string `json:"workspace,omitempty"`
	Name      string `json:"name"`
}

type branchDeleteReq struct {
	Workspace string `json:"workspace,omitempty"`
	Name      string `json:"name"`
}

type fileEntry struct {
	Path       string `json:"path"`
	OrigPath   string `json:"orig_path,omitempty"`
	Status     string `json:"status"`
	Insertions int    `json:"insertions,omitempty"`
	Deletions  int    `json:"deletions,omitempty"`
	Binary     bool   `json:"binary,omitempty"`
	Conflict   bool   `json:"conflict,omitempty"`
}

type statusResp struct {
	Workspace  string      `json:"workspace"`
	Repo       bool        `json:"repo"`
	Root       string      `json:"root,omitempty"`
	Branch     string      `json:"branch,omitempty"`
	Upstream   string      `json:"upstream,omitempty"`
	Ahead      int         `json:"ahead,omitempty"`
	Behind     int         `json:"behind,omitempty"`
	Detached   bool        `json:"detached,omitempty"`
	Unborn     bool        `json:"unborn,omitempty"`
	Staged     []fileEntry `json:"staged"`
	Unstaged   []fileEntry `json:"unstaged"`
	Conflicted int         `json:"conflicted,omitempty"`
}

type branchInfo struct {
	Name        string `json:"name"`
	Current     bool   `json:"current,omitempty"`
	Upstream    string `json:"upstream,omitempty"`
	Subject     string `json:"subject,omitempty"`
	CommittedAt int64  `json:"committed_at,omitempty"`
}

type branchesResp struct {
	Workspace string       `json:"workspace"`
	Current   string       `json:"current,omitempty"`
	Detached  bool         `json:"detached,omitempty"`
	Branches  []branchInfo `json:"branches"`
}

type diffResp struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
	Staged    bool   `json:"staged"`
	Untracked bool   `json:"untracked,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Diff      string `json:"diff"`
}

type actionResp struct {
	Workspace string `json:"workspace"`
	Branch    string `json:"branch,omitempty"`
	OK        bool   `json:"ok"`
}

type lineStat struct {
	Insertions int
	Deletions  int
	Binary     bool
}
