package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type writeArgs struct {
	Path           string `json:"path"`
	Content        string `json:"content"`
	CreateDirs     *bool  `json:"create_dirs,omitempty"`
	Overwrite      *bool  `json:"overwrite,omitempty"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
}

func (p *plugin) write(ctx context.Context, req *providerRequest, args *writeArgs) (toolResult, error) {
	if err := checkContext(ctx); err != nil {
		return toolResult{}, err
	}
	if len(args.Content) > maxWriteBytes {
		return toolResult{}, badArguments("content exceeds %d bytes", maxWriteBytes)
	}
	if !utf8.ValidString(args.Content) || strings.IndexByte(args.Content, 0) >= 0 {
		return toolResult{}, badArguments("content must be UTF-8 text without NUL")
	}
	if args.ExpectedSHA256 != "" && !validSHA256(args.ExpectedSHA256) {
		return toolResult{}, badArguments("expected_sha256 must be 64 hex characters")
	}
	createDirs := true
	if args.CreateDirs != nil {
		createDirs = *args.CreateDirs
	}
	overwrite := true
	if args.Overwrite != nil {
		overwrite = *args.Overwrite
	}

	workspace, err := resolveWorkspace(req.Workspace)
	if err != nil {
		return toolResult{}, err
	}
	path, err := resolvePath(workspace, args.Path)
	if err != nil {
		return toolResult{}, err
	}

	existing, statErr := os.Stat(path)
	if statErr == nil && existing.IsDir() {
		return failure("path_is_directory", "write: path is a directory: "+args.Path), nil
	}
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fileError("write", args.Path, statErr)
	}
	if args.ExpectedSHA256 != "" {
		if statErr != nil {
			return failure("file_conflict", "write: expected_sha256 was provided but the file does not exist"), nil
		}
		current, err := fileSHA256(path)
		if err != nil {
			return fileError("write", args.Path, err)
		}
		if !strings.EqualFold(current, args.ExpectedSHA256) {
			return failure("file_conflict", "write: file changed; sha256 does not match expected_sha256"), nil
		}
	}
	if statErr == nil && !overwrite {
		return failure("file_exists", "write: file exists and overwrite is false: "+args.Path), nil
	}

	parent := filepath.Dir(path)
	parentInfo, err := os.Stat(parent)
	switch {
	case err == nil && !parentInfo.IsDir():
		return failure("path_not_directory", "write: parent path is not a directory: "+filepath.Dir(args.Path)), nil
	case err != nil && errors.Is(err, os.ErrNotExist):
		if !createDirs {
			return failure("path_not_found", "write: parent directory does not exist: "+filepath.Dir(args.Path)), nil
		}
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fileError("write", args.Path, err)
		}
	case err != nil:
		return fileError("write", args.Path, err)
	}

	perm := fs.FileMode(0o644)
	if existing != nil {
		perm = existing.Mode().Perm()
	}
	if err := writeFileAtomic(path, []byte(args.Content), perm); err != nil {
		return fileError("write", args.Path, err)
	}
	sum := sha256.Sum256([]byte(args.Content))
	meta := map[string]any{
		"path":          args.Path,
		"bytes_written": len(args.Content),
		"sha256":        hex.EncodeToString(sum[:]),
	}
	return success(fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path), meta), nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
