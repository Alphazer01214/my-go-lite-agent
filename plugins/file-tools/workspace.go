package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	codeWorkspaceRequired = "workspace_required"
	codeWorkspaceInvalid  = "workspace_invalid"
	codePathInvalid       = "path_invalid"
	codePathEscape        = "path_escape"
)

func resolveWorkspace(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", pluginsdk.ErrCode(codeWorkspaceRequired, "workspace is required")
	}
	if !filepath.IsAbs(raw) {
		return "", pluginsdk.ErrCode(codeWorkspaceInvalid, "workspace must be an absolute path")
	}
	absolute, err := filepath.Abs(raw)
	if err != nil {
		return "", pluginsdk.ErrCode(codeWorkspaceInvalid, err.Error())
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() {
		return "", pluginsdk.ErrCode(codeWorkspaceInvalid, "workspace is not a directory: "+absolute)
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = resolved
	}
	return filepath.Clean(absolute), nil
}

func resolvePath(workspace, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", pluginsdk.ErrCode(codePathInvalid, "path is required")
	}
	if strings.ContainsRune(raw, 0) {
		return "", pluginsdk.ErrCode(codePathInvalid, "path contains NUL")
	}
	if filepath.IsAbs(raw) || filepath.VolumeName(raw) != "" {
		return "", pluginsdk.ErrCode(codePathInvalid, "path must be relative to the workspace")
	}
	cleaned := filepath.Clean(raw)
	if cleaned == "." {
		cleaned = ""
	}
	if escapesWorkspace(cleaned) {
		return "", pluginsdk.ErrCode(codePathEscape, "path escapes the workspace")
	}
	full := filepath.Join(workspace, cleaned)
	if err := ensureWithinWorkspace(workspace, full); err != nil {
		return "", err
	}
	return full, nil
}

func ensureWithinWorkspace(workspace, full string) error {
	rel, err := filepath.Rel(workspace, full)
	if err != nil {
		return pluginsdk.ErrCode(codePathEscape, err.Error())
	}
	if escapesWorkspace(rel) {
		return pluginsdk.ErrCode(codePathEscape, "path escapes the workspace")
	}
	ancestor, err := nearestExistingAncestor(full)
	if err != nil {
		return nil
	}
	resolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return nil
	}
	resolvedRel, err := filepath.Rel(workspace, resolved)
	if err != nil || escapesWorkspace(resolvedRel) {
		return pluginsdk.ErrCode(codePathEscape, "path resolves outside the workspace")
	}
	if _, err := os.Lstat(full); err == nil {
		resolvedFull, err := filepath.EvalSymlinks(full)
		if err != nil {
			return nil
		}
		resolvedRel, err = filepath.Rel(workspace, resolvedFull)
		if err != nil || escapesWorkspace(resolvedRel) {
			return pluginsdk.ErrCode(codePathEscape, "path resolves outside the workspace")
		}
	}
	return nil
}

func escapesWorkspace(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func nearestExistingAncestor(path string) (string, error) {
	current := path
	for {
		if _, err := os.Lstat(current); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		current = parent
	}
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeFileAtomic(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".liteagent-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	_ = os.Chmod(tempPath, perm)
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

func fileError(op, path string, err error) (toolResult, error) {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return failure("path_not_found", fmt.Sprintf("%s: file not found: %s", op, path)), nil
	case errors.Is(err, os.ErrPermission):
		return failure("file_io", fmt.Sprintf("%s: permission denied: %s", op, path)), nil
	default:
		return failure("file_io", fmt.Sprintf("%s: %v", op, err)), nil
	}
}
