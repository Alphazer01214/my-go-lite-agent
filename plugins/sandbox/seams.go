package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type seamKey struct {
	Capability string `json:"capability"`
	Method     string `json:"method"`
}

var defaultSeams = []seamKey{
	{Capability: "tool", Method: "call"},
	{Capability: "file", Method: "write"},
	{Capability: "file", Method: "edit"},
	{Capability: "shell", Method: "execute"},
}

// loadManifestSeams reads the plugin's own manifest so the registered handler
// set always matches the Host discovery used by the pipeline.
func loadManifestSeams(path string) []seamKey {
	data, err := os.ReadFile(path)
	if err != nil {
		logf("read manifest: %v", err)
		return append([]seamKey(nil), defaultSeams...)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var manifest struct {
		Consumes []seamKey `json:"consumes"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		logf("parse manifest: %v", err)
		return append([]seamKey(nil), defaultSeams...)
	}
	seen := make(map[string]bool, len(manifest.Consumes))
	seams := make([]seamKey, 0, len(manifest.Consumes))
	for _, seam := range manifest.Consumes {
		seam.Capability = strings.TrimSpace(seam.Capability)
		seam.Method = strings.TrimSpace(seam.Method)
		if seam.Capability == "" || seam.Method == "" {
			continue
		}
		key := seam.Capability + "." + seam.Method
		if seen[key] {
			continue
		}
		seen[key] = true
		seams = append(seams, seam)
	}
	if len(seams) == 0 {
		logf("manifest has no consumes, using defaults")
		return append([]seamKey(nil), defaultSeams...)
	}
	return seams
}

func (s seamKey) String() string {
	return fmt.Sprintf("%s.%s", s.Capability, s.Method)
}
