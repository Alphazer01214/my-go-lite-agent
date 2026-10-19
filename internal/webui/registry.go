package webui

import (
	"net/url"
	"os"
	urlpath "path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tomori/my-go-lite-agent/internal/host"
	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

const (
	diagnosticUIPluginDuplicate = "ui_plugin_duplicate"
	diagnosticUIEntryInvalid    = "ui_entry_invalid"
	diagnosticUIEntryMissing    = "ui_entry_missing"
	diagnosticUIEntryOutside    = "ui_entry_outside_package"
	diagnosticUIResourceRoot    = "ui_resource_root_invalid"
	diagnosticUITagInvalid      = "ui_tag_invalid"
	diagnosticUITagPrefix       = "ui_tag_prefix_mismatch"
	diagnosticUIMountIDInvalid  = "ui_mount_id_invalid"
	diagnosticUIMountDuplicate  = "ui_mount_duplicate"
	diagnosticUISlotInvalid     = "ui_slot_invalid"
	diagnosticUISlotReserved    = "ui_slot_reserved"
	diagnosticUISlotDuplicate   = "ui_slot_duplicate"
)

var (
	customElementPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]*-[a-z0-9._-]+$`)
	mountIDPattern       = regexp.MustCompile(`^[a-z0-9_-]+$`)
)

type Registry struct {
	mounts        map[MountKey]MountSpec
	order         []MountKey
	plugins       map[string]BootstrapPlugin
	pluginOrder   []string
	resourceRoots map[string]string
	pluginRoots   map[string]string
	diagnostics   []plugin.Diagnostic
}

func NewRegistry(discoveries plugin.Discoveries) (*Registry, []plugin.Diagnostic) {
	registry := &Registry{
		mounts:        make(map[MountKey]MountSpec),
		plugins:       make(map[string]BootstrapPlugin),
		resourceRoots: make(map[string]string),
		pluginRoots:   make(map[string]string),
		diagnostics:   append([]plugin.Diagnostic(nil), discoveries.Diagnostics...),
	}

	seenPlugins := make(map[string]struct{})
	seenMountIDs := make(map[string]map[string]struct{})
	seenSlots := make(map[string]MountKey)

	for _, discovery := range discoveries.Plugins {
		pluginName := discovery.Manifest.Name
		if pluginName == "" {
			pluginName = filepath.Base(discovery.Dir)
		}
		if _, exists := seenPlugins[pluginName]; exists {
			registry.addDiagnostic(plugin.Diagnostic{
				Plugin: pluginName, Path: filepath.Join(discovery.Dir, "plugin.json"),
				Code: diagnosticUIPluginDuplicate, Message: "plugin name is already registered",
			})
			continue
		}
		seenPlugins[pluginName] = struct{}{}
		seenMountIDs[pluginName] = make(map[string]struct{})
		registry.plugins[pluginName] = BootstrapPlugin{PluginSnapshot: host.PluginSnapshot{
			Name:      pluginName,
			Version:   discovery.Manifest.Version,
			State:     host.ProviderStateInactive,
			Enabled:   true,
			Provides:  append([]string(nil), discovery.Manifest.Provides...),
			DependsOn: append([]string(nil), discovery.Manifest.DependsOn...),
		},
			Settings: discovery.Manifest.Settings,
		}
		registry.pluginOrder = append(registry.pluginOrder, pluginName)

		if discovery.Manifest.UI == nil {
			continue
		}

		sourcePath := filepath.Join(discovery.Dir, "plugin.json")
		entryPath, entryURL, entryReady := registry.resolveEntry(discovery.Dir, discovery.Manifest.UI.Entry, pluginName, sourcePath)
		if !entryReady {
			// resolveEntry already reported why; the plugin keeps its manifest
			// entry but contributes no mount.
			continue
		}
		registry.pluginRoots[pluginName] = discovery.Dir
		registry.resourceRoots[pluginName] = filepath.Dir(entryPath)

		for i, component := range discovery.Manifest.UI.Components {
			componentPath := sourcePath + "#ui.components[" + strconv.Itoa(i) + "]"
			spec, ok := registry.buildSpec(
				pluginName, component, entryURL, componentPath,
				seenMountIDs[pluginName], seenSlots,
			)
			if !ok {
				continue
			}
			registry.mounts[spec.Key] = spec
			registry.order = append(registry.order, spec.Key)
		}
	}

	sort.Strings(registry.pluginOrder)
	return registry, append([]plugin.Diagnostic(nil), registry.diagnostics...)
}

func (r *Registry) resolveEntry(pluginDir, entry, pluginName, sourcePath string) (string, string, bool) {
	relativePath, ok := cleanRelativePath(entry)
	if !ok {
		code := diagnosticUIEntryInvalid
		message := "ui.entry must be a package-relative path"
		if entry != "" {
			code = diagnosticUIEntryOutside
			message = "ui.entry must stay inside the plugin package"
		}
		r.addDiagnostic(plugin.Diagnostic{
			Plugin: pluginName, Path: sourcePath + "#ui.entry", Code: code, Message: message,
		})
		return "", "", false
	}

	root, err := filepath.Abs(pluginDir)
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err != nil {
		r.addDiagnostic(plugin.Diagnostic{
			Plugin: pluginName, Path: sourcePath + "#ui.entry", Code: diagnosticUIResourceRoot,
			Message: err.Error(),
		})
		return "", "", false
	}

	entryPath := filepath.Join(root, filepath.FromSlash(relativePath))
	info, err := os.Stat(entryPath)
	if err != nil {
		code := diagnosticUIEntryMissing
		if !os.IsNotExist(err) {
			code = diagnosticUIResourceRoot
		}
		r.addDiagnostic(plugin.Diagnostic{
			Plugin: pluginName, Path: sourcePath + "#ui.entry", Code: code, Message: err.Error(),
		})
		return "", "", false
	}
	if !info.Mode().IsRegular() {
		r.addDiagnostic(plugin.Diagnostic{
			Plugin: pluginName, Path: sourcePath + "#ui.entry", Code: diagnosticUIEntryInvalid,
			Message: "ui.entry must reference a regular file",
		})
		return "", "", false
	}

	realEntry, err := filepath.EvalSymlinks(entryPath)
	if err != nil || !withinRoot(root, realEntry) {
		message := "ui.entry resolves outside the plugin package"
		if err != nil {
			message = err.Error()
		}
		r.addDiagnostic(plugin.Diagnostic{
			Plugin: pluginName, Path: sourcePath + "#ui.entry", Code: diagnosticUIEntryOutside, Message: message,
		})
		return "", "", false
	}

	return realEntry, pluginResourceURL(pluginName, relativePath), true
}

func (r *Registry) buildSpec(
	pluginName string,
	component plugin.UIComponent,
	entryURL, componentPath string,
	seenMountIDs map[string]struct{},
	seenSlots map[string]MountKey,
) (MountSpec, bool) {
	valid := true

	if !customElementPattern.MatchString(component.Tag) {
		r.addDiagnostic(plugin.Diagnostic{
			Plugin: pluginName, Path: componentPath, Code: diagnosticUITagInvalid,
			Message: "tag is not a valid Custom Element name",
		})
		valid = false
	} else if !strings.HasPrefix(component.Tag, pluginName+"-") || component.Tag == pluginName+"-" {
		r.addDiagnostic(plugin.Diagnostic{
			Plugin: pluginName, Path: componentPath, Code: diagnosticUITagPrefix,
			Message: "tag must start with <plugin_name>-",
		})
		valid = false
	}

	if !mountIDPattern.MatchString(component.MountID) {
		r.addDiagnostic(plugin.Diagnostic{
			Plugin: pluginName, Path: componentPath, Code: diagnosticUIMountIDInvalid,
			Message: "mount_id must match [a-z0-9_-]+",
		})
		valid = false
	} else if _, exists := seenMountIDs[component.MountID]; exists {
		r.addDiagnostic(plugin.Diagnostic{
			Plugin: pluginName, Path: componentPath, Code: diagnosticUIMountDuplicate,
			Message: "mount_id is already registered",
		})
		valid = false
	}

	switch component.Slot {
	case "top":
		r.addDiagnostic(plugin.Diagnostic{
			Plugin: pluginName, Path: componentPath, Code: diagnosticUISlotReserved,
			Message: "top is reserved for Host controls",
		})
		valid = false
	case "left", "center", "right", "bottom":
	default:
		r.addDiagnostic(plugin.Diagnostic{
			Plugin: pluginName, Path: componentPath, Code: diagnosticUISlotInvalid,
			Message: "slot must be left, center, right, or bottom",
		})
		valid = false
	}

	if !valid {
		return MountSpec{}, false
	}
	if owner, exists := seenSlots[component.Slot]; exists {
		r.addDiagnostic(plugin.Diagnostic{
			Plugin: pluginName, Path: componentPath, Code: diagnosticUISlotDuplicate,
			Message: "slot is already used by " + owner.Plugin + ":" + owner.ID,
		})
		return MountSpec{}, false
	}

	key := MountKey{Plugin: pluginName, ID: component.MountID}
	seenMountIDs[component.MountID] = struct{}{}
	seenSlots[component.Slot] = key
	return MountSpec{Key: key, Tag: component.Tag, Slot: component.Slot, EntryURL: entryURL}, true
}

// Replace is intentionally absent: a Registry is immutable once built. Swapping
// its contents in place would let a concurrent reader (which only borrows the
// pointer) observe a half-written map. Callers publish a new *Registry instead.
func (r *Registry) Bootstrap() Bootstrap {
	mounts := make([]BootstrapMount, 0, len(r.order))
	for _, key := range r.order {
		spec := r.mounts[key]
		mounts = append(mounts, BootstrapMount{
			Mount: MountRef{Plugin: key.Plugin, ID: key.ID},
			Tag:   spec.Tag, Slot: spec.Slot, Entry: spec.EntryURL,
		})
	}
	plugins := make([]BootstrapPlugin, 0, len(r.pluginOrder))
	pluginIndex := make(map[string]int, len(r.pluginOrder))
	for _, name := range r.pluginOrder {
		pluginIndex[name] = len(plugins)
		plugins = append(plugins, r.plugins[name])
	}
	for _, mount := range mounts {
		if i, ok := pluginIndex[mount.Mount.Plugin]; ok {
			plugins[i].Mounts = append(plugins[i].Mounts, mount.Mount)
		}
	}

	return Bootstrap{
		Diagnostics: append(make([]plugin.Diagnostic, 0, len(r.diagnostics)), r.diagnostics...),
		Version:     1,
		Slots:       append([]string(nil), slots...),
		Plugins:     plugins,
		Mounts:      mounts,
	}
}

func (r *Registry) Mounts() []MountSpec {
	mounts := make([]MountSpec, 0, len(r.order))
	for _, key := range r.order {
		mounts = append(mounts, r.mounts[key])
	}
	return mounts
}

func (r *Registry) Lookup(key MountKey) (MountSpec, bool) {
	spec, ok := r.mounts[key]
	return spec, ok
}

func (r *Registry) ResourceRoot(pluginName string) (string, bool) {
	root, ok := r.resourceRoots[pluginName]
	return root, ok
}

func (r *Registry) ResolveResource(pluginName, packagePath string) (string, bool) {
	pluginRoot, ok := r.pluginRoots[pluginName]
	if !ok {
		return "", false
	}
	resourceRoot, ok := r.resourceRoots[pluginName]
	if !ok {
		return "", false
	}
	cleaned, ok := cleanRelativePath(packagePath)
	if !ok {
		return "", false
	}
	candidate := filepath.Join(pluginRoot, filepath.FromSlash(cleaned))
	realPath, err := filepath.EvalSymlinks(candidate)
	if err != nil || !withinRoot(resourceRoot, realPath) {
		return "", false
	}
	info, err := os.Stat(realPath)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return realPath, true
}

func (r *Registry) addDiagnostic(diagnostic plugin.Diagnostic) {
	r.diagnostics = append(r.diagnostics, diagnostic)
}

func cleanRelativePath(value string) (string, bool) {
	if value == "" {
		return "", false
	}
	forwardSlash := strings.ReplaceAll(value, "\\", "/")
	if urlpath.IsAbs(forwardSlash) || filepath.IsAbs(value) {
		return "", false
	}
	for _, part := range strings.Split(forwardSlash, "/") {
		if part == ".." {
			return "", false
		}
	}
	cleaned := urlpath.Clean(forwardSlash)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

func withinRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func pluginResourceURL(pluginName, relativePath string) string {
	parts := []string{"plugins", url.PathEscape(pluginName)}
	for _, part := range strings.Split(filepath.ToSlash(relativePath), "/") {
		parts = append(parts, url.PathEscape(part))
	}
	return "/" + strings.Join(parts, "/")
}
