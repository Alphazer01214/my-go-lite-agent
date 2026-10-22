package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type workspace struct {
	root *os.Root
	path string
}

func openWorkspace(path string) (*workspace, error) {
	if path == "" {
		return nil, errWorkspaceRequired("workspace is required")
	}
	if !filepath.IsAbs(path) {
		return nil, errWorkspaceInvalid("workspace must be an absolute path")
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return nil, errWorkspaceInvalid("workspace must be an existing directory")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, errWorkspaceInvalid(err.Error())
	}
	return &workspace{root: root, path: path}, nil
}

func (w *workspace) Close() error {
	if w == nil || w.root == nil {
		return nil
	}
	return w.root.Close()
}

func safeRelative(path string, allowRoot bool) (string, error) {
	if path == "" {
		return "", errPathInvalid("path is required")
	}
	if strings.ContainsRune(path, 0) {
		return "", errPathInvalid("path contains NUL")
	}
	normalized := strings.ReplaceAll(path, `\`, "/")
	if strings.HasPrefix(normalized, "/") || strings.HasPrefix(normalized, "//") ||
		filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return "", errPathInvalid("path must be workspace-relative")
	}
	if normalized == "." {
		if allowRoot {
			return ".", nil
		}
		return "", errPathInvalid("a file path is required")
	}
	parts := strings.Split(normalized, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", errPathInvalid("path contains an invalid component")
		}
		if windowsReservedName(part) {
			return "", errPathInvalid("path uses a reserved device name")
		}
	}
	return filepath.FromSlash(strings.Join(parts, "/")), nil
}

func windowsReservedName(name string) bool {
	base := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(base) == 4 {
		prefix := base[:3]
		if (prefix == "COM" || prefix == "LPT") && base[3] >= '1' && base[3] <= '9' {
			return true
		}
	}
	return false
}

func mapPathError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return errPathNotFound(err.Error())
	}
	if errors.Is(err, fs.ErrPermission) {
		return errPathEscape(err.Error())
	}
	return errFileIo(err.Error())
}

func randomSuffix() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "tmp"
	}
	return hex.EncodeToString(value[:])
}

func atomicWrite(w *workspace, rel string, content []byte, perm fs.FileMode) error {
	dir := filepath.Dir(rel)
	if dir != "." {
		if err := w.root.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := filepath.Join(dir, ".liteagent-"+randomSuffix()+".tmp")
	file, err := w.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		_ = file.Close()
		if cleanup {
			_ = w.root.Remove(tmp)
		}
	}()
	if _, err := file.Write(content); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := w.root.Rename(tmp, rel); err != nil {
		return err
	}
	cleanup = false
	return nil
}
