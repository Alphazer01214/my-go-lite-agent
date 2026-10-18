package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

type statIn struct {
	Path string `json:"path"`
	Hash *bool  `json:"hash,omitempty"`
}

func (p *plugin) stat(_ context.Context, _ *pluginsdk.Request, input toolcontract.ProviderInput, arguments json.RawMessage) (json.RawMessage, error) {
	var in statIn
	if err := toolcontract.DecodeStrict(arguments, &in); err != nil {
		return nil, err
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
	info, err := w.root.Lstat(rel)
	if err != nil {
		return nil, mapPathError(err)
	}
	entryType := "file"
	switch {
	case info.IsDir():
		entryType = "dir"
	case info.Mode()&fs.ModeSymlink != 0:
		entryType = "symlink"
		if _, err := w.root.Stat(rel); err != nil {
			return nil, mapPathError(err)
		}
	}
	meta := map[string]any{
		"path":       displayPath(rel),
		"type":       entryType,
		"size":       info.Size(),
		"mode":       info.Mode().String(),
		"mtime":      info.ModTime().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		"is_symlink": info.Mode()&os.ModeSymlink != 0,
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if target, err := w.root.Readlink(rel); err == nil {
			meta["link_target"] = target
		}
	}
	if in.Hash != nil && *in.Hash && info.Mode().IsRegular() && info.Size() <= 8<<20 {
		if data, err := w.root.ReadFile(rel); err == nil {
			meta["sha256"] = hashBytes(data)
		}
	}
	return toolcontract.Success(humanContent(meta), meta)
}
