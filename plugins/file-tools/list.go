package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type listIn struct {
	Path          string `json:"path,omitempty"`
	Recursive     *bool  `json:"recursive,omitempty"`
	MaxDepth      *int   `json:"max_depth,omitempty"`
	MaxEntries    *int   `json:"max_entries,omitempty"`
	IncludeHidden *bool  `json:"include_hidden,omitempty"`
}

type entryOut struct {
	Path  string `json:"path"`
	Type  string `json:"type"`
	Size  int64  `json:"size"`
	MTime string `json:"mtime"`
}

func (p *plugin) list(_ context.Context, _ *pluginsdk.Request, input ProviderInput, arguments json.RawMessage) (json.RawMessage, error) {
	var in listIn
	if err := DecodeStrict(arguments, &in); err != nil {
		return nil, err
	}
	if in.Path == "" {
		in.Path = "."
	}
	w, err := openWorkspace(input.Workspace)
	if err != nil {
		return nil, err
	}
	defer w.Close()
	rel, err := safeRelative(in.Path, true)
	if err != nil {
		return nil, err
	}
	info, err := w.root.Stat(rel)
	if err != nil {
		return nil, mapPathError(err)
	}
	if !info.IsDir() {
		return nil, pluginsdk.ErrCode("path_not_directory", "path is not a directory")
	}
	recursive := in.Recursive != nil && *in.Recursive
	maxDepth := 1
	if recursive {
		maxDepth = 4
	}
	if in.MaxDepth != nil {
		maxDepth = *in.MaxDepth
	}
	maxEntries := 200
	if in.MaxEntries != nil {
		maxEntries = *in.MaxEntries
	}
	includeHidden := in.IncludeHidden == nil || *in.IncludeHidden
	entries, truncated, err := walkEntries(w, rel, recursive, maxDepth, maxEntries, includeHidden)
	if err != nil {
		return nil, mapPathError(err)
	}
	contentLines := make([]string, 0, len(entries))
	for _, entry := range entries {
		contentLines = append(contentLines, entry.Type+"\t"+entry.Path+"\t"+
			strconv.FormatInt(entry.Size, 10)+"\t"+entry.MTime)
	}
	return Encode(Result{
		Content: strings.Join(contentLines, "\n"), Truncated: truncated,
		Meta: map[string]any{"path": displayPath(rel), "entries": entries},
	})
}

func walkEntries(w *workspace, base string, recursive bool, maxDepth, maxEntries int, includeHidden bool) ([]entryOut, bool, error) {
	var out []entryOut
	var walk func(string, int) error
	walk = func(dir string, depth int) error {
		file, err := w.root.Open(dir)
		if err != nil {
			return err
		}
		items, err := file.ReadDir(-1)
		_ = file.Close()
		if err != nil {
			return err
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Name() < items[j].Name() })
		for _, item := range items {
			if !includeHidden && strings.HasPrefix(item.Name(), ".") {
				continue
			}
			child := joinRelative(displayPath(dir), item.Name())
			info, err := item.Info()
			if err != nil {
				return err
			}
			entryType := "file"
			switch {
			case info.IsDir():
				entryType = "dir"
			case info.Mode()&fs.ModeSymlink != 0:
				entryType = "symlink"
			}
			out = append(out, entryOut{
				Path: child, Type: entryType, Size: info.Size(), MTime: info.ModTime().UTC().Format(time.RFC3339Nano),
			})
			if len(out) > maxEntries {
				return nil
			}
			if recursive && info.IsDir() && depth < maxDepth {
				if err := walk(filepath.FromSlash(child), depth+1); err != nil {
					return err
				}
				if len(out) > maxEntries {
					return nil
				}
			}
		}
		return nil
	}
	err := walk(base, 1)
	truncated := len(out) > maxEntries
	if truncated {
		out = out[:maxEntries]
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, truncated, err
}
