package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"unicode/utf8"
)

type editArgs struct {
	Path           string `json:"path"`
	OldText        string `json:"old_text"`
	NewText        string `json:"new_text"`
	ReplaceAll     bool   `json:"replace_all,omitempty"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
}

func (p *plugin) edit(ctx context.Context, req *providerRequest, args *editArgs) (toolResult, error) {
	if err := checkContext(ctx); err != nil {
		return toolResult{}, err
	}
	if args.OldText == "" {
		return toolResult{}, badArguments("old_text is required")
	}
	if !utf8.ValidString(args.OldText) || !utf8.ValidString(args.NewText) {
		return toolResult{}, badArguments("old_text and new_text must be valid UTF-8")
	}
	if args.ExpectedSHA256 != "" && !validSHA256(args.ExpectedSHA256) {
		return toolResult{}, badArguments("expected_sha256 must be 64 hex characters")
	}

	workspace, err := resolveWorkspace(req.Workspace)
	if err != nil {
		return toolResult{}, err
	}
	path, err := resolvePath(workspace, args.Path)
	if err != nil {
		return toolResult{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fileError("edit", args.Path, err)
	}
	if info.IsDir() {
		return failure("path_is_directory", "edit: path is a directory: "+args.Path), nil
	}

	data, err := readTextFile(path)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return failure("path_not_found", "edit: file not found: "+args.Path), nil
		case errors.Is(err, errPathIsDirectory):
			return failure("path_is_directory", "edit: path is a directory: "+args.Path), nil
		case errors.Is(err, errFileTooLarge):
			return failure("file_too_large", "edit: "+err.Error()), nil
		case errors.Is(err, errFileBinary):
			return failure("file_binary", "edit: file is not UTF-8 text: "+args.Path), nil
		default:
			return fileError("edit", args.Path, err)
		}
	}
	if args.ExpectedSHA256 != "" {
		sum := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), args.ExpectedSHA256) {
			return failure("file_conflict", "edit: file changed; sha256 does not match expected_sha256"), nil
		}
	}

	content := string(data)
	occurrences := strings.Count(content, args.OldText)
	if occurrences == 0 {
		return failure("edit_not_found", "edit: old_text was not found"), nil
	}
	if occurrences > 1 && !args.ReplaceAll {
		return failure("edit_ambiguous", fmt.Sprintf("edit: old_text matches %d times; set replace_all to true or provide more context", occurrences)), nil
	}
	replacements := 1
	if args.ReplaceAll {
		replacements = occurrences
	}
	updated := strings.Replace(content, args.OldText, args.NewText, replacements)
	if len(updated) > maxEditFileBytes {
		return failure("file_too_large", fmt.Sprintf("edit: result exceeds %d bytes", maxEditFileBytes)), nil
	}
	if err := writeFileAtomic(path, []byte(updated), fs.FileMode(info.Mode().Perm())); err != nil {
		return fileError("edit", args.Path, err)
	}

	sum := sha256.Sum256([]byte(updated))
	meta := map[string]any{
		"path":          args.Path,
		"replacements":  replacements,
		"bytes_written": len(updated),
		"sha256":        hex.EncodeToString(sum[:]),
	}
	return success(fmt.Sprintf("replaced %d occurrence(s) in %s", replacements, args.Path), meta), nil
}
