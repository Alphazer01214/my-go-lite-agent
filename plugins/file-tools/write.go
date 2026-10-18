package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

const maxWriteBytes = 4 << 20

type writeIn struct {
	Path           string  `json:"path"`
	Content        string  `json:"content"`
	CreateDirs     *bool   `json:"create_dirs,omitempty"`
	Overwrite      *bool   `json:"overwrite,omitempty"`
	ExpectedSHA256 *string `json:"expected_sha256,omitempty"`
}

func (p *plugin) write(_ context.Context, _ *pluginsdk.Request, input toolcontract.ProviderInput, arguments json.RawMessage) (json.RawMessage, error) {
	var in writeIn
	if err := toolcontract.DecodeStrict(arguments, &in); err != nil {
		return nil, err
	}
	if len(in.Content) > maxWriteBytes {
		return nil, pluginsdk.ErrCode("file_too_large", "content exceeds 4 MiB")
	}
	w, err := openWorkspace(input.Workspace)
	if err != nil {
		return nil, err
	}
	defer w.Close()
	rel, err := safeRelative(in.Path, false)
	if err != nil {
		return nil, err
	}
	createDirs := in.CreateDirs != nil && *in.CreateDirs
	overwrite := in.Overwrite == nil || *in.Overwrite
	exists := false
	existing, readErr := w.root.ReadFile(rel)
	if readErr == nil {
		exists = true
	} else if !os.IsNotExist(readErr) {
		info, statErr := w.root.Stat(rel)
		if statErr == nil && info.IsDir() {
			return nil, pluginsdk.ErrCode("path_is_directory", "path is a directory")
		}
		return nil, mapPathError(readErr)
	}
	if exists && !overwrite {
		return toolcontract.Failure("file_exists", "file already exists", map[string]any{"path": displayPath(rel)})
	}
	if in.ExpectedSHA256 != nil {
		if !exists {
			return toolcontract.Failure("file_conflict", "expected file does not exist", map[string]any{"path": displayPath(rel)})
		}
		expected := strings.ToLower(*in.ExpectedSHA256)
		if hashBytes(existing) != expected {
			return toolcontract.Failure("file_conflict", "file digest does not match", map[string]any{"path": displayPath(rel)})
		}
	}
	if !createDirs {
		if _, err := w.root.Stat(filepath.Dir(rel)); err != nil {
			if filepath.Dir(rel) == "." {
				err = nil
			}
			if err != nil {
				return nil, mapPathError(err)
			}
		}
	}
	if err := atomicWrite(w, rel, []byte(in.Content), 0o644); err != nil {
		return nil, mapPathError(err)
	}
	return toolcontract.Success("", map[string]any{
		"path":          displayPath(rel),
		"bytes_written": len(in.Content),
		"sha256":        hashBytes([]byte(in.Content)),
		"created":       !exists,
	})
}

type editIn struct {
	OldText    string `json:"old_text"`
	NewText    string `json:"new_text"`
	ReplaceAll *bool  `json:"replace_all,omitempty"`
}

type replaceIn struct {
	Path           string  `json:"path"`
	OldText        string  `json:"old_text"`
	NewText        string  `json:"new_text"`
	ReplaceAll     *bool   `json:"replace_all,omitempty"`
	ExpectedSHA256 *string `json:"expected_sha256,omitempty"`
}

func (p *plugin) replace(_ context.Context, _ *pluginsdk.Request, input toolcontract.ProviderInput, arguments json.RawMessage) (json.RawMessage, error) {
	var in replaceIn
	if err := toolcontract.DecodeStrict(arguments, &in); err != nil {
		return nil, err
	}
	replaceAll := in.ReplaceAll != nil && *in.ReplaceAll
	return p.applyEdits(input.Workspace, in.Path, in.ExpectedSHA256, []editIn{{
		OldText: in.OldText, NewText: in.NewText, ReplaceAll: &replaceAll,
	}})
}

type patchIn struct {
	Path           string   `json:"path"`
	Edits          []editIn `json:"edits"`
	ExpectedSHA256 *string  `json:"expected_sha256,omitempty"`
}

func (p *plugin) patch(_ context.Context, _ *pluginsdk.Request, input toolcontract.ProviderInput, arguments json.RawMessage) (json.RawMessage, error) {
	var in patchIn
	if err := toolcontract.DecodeStrict(arguments, &in); err != nil {
		return nil, err
	}
	if len(in.Edits) == 0 || len(in.Edits) > 32 {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "edits must contain 1 to 32 items")
	}
	return p.applyEdits(input.Workspace, in.Path, in.ExpectedSHA256, in.Edits)
}

func (p *plugin) applyEdits(workspacePath, filePath string, expected *string, edits []editIn) (json.RawMessage, error) {
	w, err := openWorkspace(workspacePath)
	if err != nil {
		return nil, err
	}
	defer w.Close()
	rel, err := safeRelative(filePath, false)
	if err != nil {
		return nil, err
	}
	data, err := w.root.ReadFile(rel)
	if err != nil {
		return nil, mapPathError(err)
	}
	if len(data) > 8<<20 {
		return nil, pluginsdk.ErrCode("file_too_large", "file exceeds 8 MiB")
	}
	if expected != nil && hashBytes(data) != strings.ToLower(*expected) {
		return toolcontract.Failure("file_conflict", "file digest does not match", map[string]any{"path": displayPath(rel)})
	}
	current := string(data)
	for _, edit := range edits {
		if edit.OldText == "" {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "old_text must be non-empty")
		}
		count := strings.Count(current, edit.OldText)
		if count == 0 {
			return toolcontract.Failure("edit_not_found", "old_text was not found", map[string]any{"path": displayPath(rel)})
		}
		replaceAll := edit.ReplaceAll != nil && *edit.ReplaceAll
		if count > 1 && !replaceAll {
			return toolcontract.Failure("edit_ambiguous", "old_text matches more than once", map[string]any{
				"path": displayPath(rel), "matches": count,
			})
		}
		replacementCount := 1
		if replaceAll {
			replacementCount = -1
		}
		current = strings.Replace(current, edit.OldText, edit.NewText, replacementCount)
	}
	if err := atomicWrite(w, rel, []byte(current), 0o644); err != nil {
		return nil, mapPathError(err)
	}
	return toolcontract.Success("", map[string]any{
		"path":    displayPath(rel),
		"edits":   len(edits),
		"sha256":  hashBytes([]byte(current)),
		"changed": current != string(data),
	})
}
