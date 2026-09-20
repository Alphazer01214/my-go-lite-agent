// // Package plugin defines the on-disk Plugin manifest.
package plugin

// import (
// 	"bytes"
// 	"encoding/json"
// 	"fmt"
// 	"os"
// 	"path"
// 	"path/filepath"
// 	"regexp"
// 	"strings"
// )

// // CommandSpec is one slash command a Plugin declares in its Manifest.
// type CommandSpec struct {
// 	Name        string `json:"name"`
// 	Description string `json:"description"`
// 	Usage       string `json:"usage"`
// }

// // UISpec declares optional Web Medium Panel components (ADR-0010, ADR-0012).
// type UISpec struct {
// 	// Entry is the plugin's UI Entry ES Module, relative to the plugin's ui/ dir.
// 	Entry string `json:"entry"`
// 	// Assets lists auxiliary files (css, html templates) the components
// 	// fetch at runtime, relative to ui/. Declared files must exist (ADR-0011).
// 	Assets []string  `json:"assets,omitempty"`
// 	Mounts []UIMount `json:"mounts,omitempty"`
// 	// Trust reserves isolation (ADR-0012). Default/only implemented value: full.
// 	Trust string `json:"trust,omitempty"`
// 	// Pages are additive page contributions (may not replace base layout pages).
// 	Pages []UIPage `json:"pages,omitempty"`
// }

// // UIPage is a plugin-contributed layout page (ADR-0012).
// type UIPage struct {
// 	Slug  string   `json:"slug"`
// 	Title string   `json:"title,omitempty"`
// 	Path  string   `json:"path"`
// 	Slots []UISlot `json:"slots"`
// }

// // UISlot is a slot declared on a contributed page.
// type UISlot struct {
// 	ID        string `json:"id"`
// 	Role      string `json:"role,omitempty"`
// 	Preferred string `json:"preferred,omitempty"`
// 	Region    string `json:"region,omitempty"`
// }

// // UIMount statically mounts one Panel Component into a Panel slot at startup.
// // Page selects the layout page (ADR-0011); empty means "main".
// type UIMount struct {
// 	Page      string          `json:"page,omitempty"`
// 	Slot      string          `json:"slot"`
// 	Component string          `json:"component"`
// 	Props     json.RawMessage `json:"props,omitempty"`
// }

// // UISlots are the Shell-hosted Panel slot names (ADR-0031): top/bottom plus
// // the left|center|right body row. Domain faces (chat/trace/…) are
// // plugin-owned components mounted into these regions — never named Shell slots.
// var UISlots = []string{"top", "bottom", "left", "center", "right"}

// // ValidUISlot reports whether slot is a known base Shell Panel slot.
// func ValidUISlot(slot string) bool {
// 	for _, s := range UISlots {
// 		if s == slot {
// 			return true
// 		}
// 	}
// 	return false
// }

// // slotPattern allows base slots and contributed-page slot ids (ADR-0012).
// var slotPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// // ValidMountSlot reports whether a mount may target this slot id.
// func ValidMountSlot(slot string) bool {
// 	if ValidUISlot(slot) {
// 		return true
// 	}
// 	return slotPattern.MatchString(slot)
// }

// // NormalizedEntry returns the slash entry path guaranteed to resolve under ui/.
// func (u UISpec) NormalizedEntry() string {
// 	clean := path.Clean(strings.ReplaceAll(u.Entry, "\\", "/"))
// 	if !strings.HasPrefix(clean, "ui/") {
// 		clean = "ui/" + clean
// 	}
// 	return clean
// }

// // Manifest is plugin.json next to a Plugin executable.
// type Manifest struct {
// 	Name     string `json:"name"`
// 	Version  string `json:"version"`
// 	Protocol int    `json:"protocol"`
// 	// Provides 插件对外提供的能力
// 	Provides []string `json:"provides"`
// 	Consumes []string `json:"consumes"`
// 	// Requires 需要的 capabilities
// 	Requires    []string      `json:"requires"`
// 	Entry       string        `json:"entry"`
// 	TimeoutMs   int           `json:"timeoutMs,omitempty"`
// 	Description string        `json:"description,omitempty"`
// 	Commands    []CommandSpec `json:"commands,omitempty"`
// 	UI          *UISpec       `json:"ui,omitempty"`
// 	// Autostart marks this Plugin as a Host startup root (ADR-0021). Default false.
// 	Autostart bool `json:"autostart,omitempty"`
// 	// DependsOn names other Plugins to pull into the mount closure (ADR-0021).
// 	DependsOn []string `json:"dependsOn,omitempty"`
// 	// HostFaces are Host-addressed faces served by this Plugin (ADR-0027):
// 	// config | commands | ui. Unlike provides they are not Capabilities —
// 	// every Plugin may implement them, and Host addresses them by plugin name.
// 	HostFaces []string `json:"hostFaces,omitempty"`
// }

// // CurrentProtocol is the highest plugin.json "protocol" (manifest + UI
// // contract, ADR-0012) this Host accepts. It is not the Frame wire version —
// // Frame.V carries protocol.Version. Protocol 4 carries hostFaces (ADR-0027).
// // Protocol 5 is L0-only Host/Medium addressing (ADR-0030).
// // Protocol 6 is Shell five-region slots (ADR-0031): top|bottom|left|center|right.
// const CurrentProtocol = 6

// // ValidHostFace reports whether name is a declared hostFace (ADR-0027).
// func ValidHostFace(name string) bool {
// 	switch name {
// 	case "config", "commands", "ui":
// 		return true
// 	}
// 	return false
// }

// var namePattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// // ReservedCommandNames are Host-native slash commands. Plugins whose name
// // collides (case-insensitive) are rejected at Assembly (ADR-0008).
// var ReservedCommandNames = map[string]bool{
// 	"help":    true,
// 	"lp":      true,
// 	"refresh": true,
// 	"exit":    true,
// }

// // ConflictsWithNativeCommand reports whether the Manifest name collides with a Host command.
// func (m Manifest) ConflictsWithNativeCommand() bool {
// 	return ReservedCommandNames[strings.ToLower(m.Name)]
// }

// // LoadManifest reads and validates a plugin.json path.
// func LoadManifest(path string) (*Manifest, error) {
// 	raw, err := os.ReadFile(path)
// 	if err != nil {
// 		return nil, fmt.Errorf("read manifest: %w", err)
// 	}
// 	// Windows editors may prefix UTF-8 JSON with a BOM; json.Unmarshal rejects it.
// 	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
// 	var m Manifest
// 	if err := json.Unmarshal(raw, &m); err != nil {
// 		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
// 	}
// 	if err := m.Validate(); err != nil {
// 		return nil, fmt.Errorf("invalid manifest %s: %w", path, err)
// 	}
// 	return &m, nil
// }

// // Validate checks required fields. Dir is the Plugin directory used to resolve Entry.
// func (m *Manifest) Validate() error {
// 	if strings.TrimSpace(m.Name) == "" {
// 		return fmt.Errorf("name is required")
// 	}
// 	if !namePattern.MatchString(m.Name) {
// 		return fmt.Errorf("name %q must match [a-z0-9-]+", m.Name)
// 	}
// 	if strings.TrimSpace(m.Version) == "" {
// 		return fmt.Errorf("version is required")
// 	}
// 	if m.Protocol < 1 || m.Protocol > CurrentProtocol {
// 		return fmt.Errorf("protocol must be 1..%d, got %d", CurrentProtocol, m.Protocol)
// 	}
// 	// UI-only Plugins are legal (ADR-0011): a Manifest must carry an executable
// 	// entry or a Web UI, never neither. A UI-only plugin has no process, so it
// 	// cannot serve capabilities.
// 	if strings.TrimSpace(m.Entry) == "" && m.UI == nil {
// 		return fmt.Errorf("entry or ui is required")
// 	}
// 	if strings.TrimSpace(m.Entry) == "" && len(m.Provides) > 0 {
// 		return fmt.Errorf("ui-only plugin (no entry) cannot provide capabilities")
// 	}
// 	for i, c := range m.Provides {
// 		if strings.TrimSpace(c) == "" {
// 			return fmt.Errorf("provides[%d] is empty", i)
// 		}
// 	}
// 	for i, c := range m.Consumes {
// 		if strings.TrimSpace(c) == "" {
// 			return fmt.Errorf("consumes[%d] is empty", i)
// 		}
// 	}
// 	seenFaces := map[string]bool{}
// 	for i, f := range m.HostFaces {
// 		if !ValidHostFace(f) {
// 			return fmt.Errorf("hostFaces[%d] %q must be config|commands|ui", i, f)
// 		}
// 		if seenFaces[f] {
// 			return fmt.Errorf("hostFaces[%d] %q duplicated", i, f)
// 		}
// 		seenFaces[f] = true
// 	}
// 	for i, d := range m.DependsOn {
// 		if strings.TrimSpace(d) == "" {
// 			return fmt.Errorf("dependsOn[%d] is empty", i)
// 		}
// 		if !namePattern.MatchString(d) {
// 			return fmt.Errorf("dependsOn[%d] %q must match [a-z0-9-]+", i, d)
// 		}
// 		if d == m.Name {
// 			return fmt.Errorf("dependsOn[%d] %q must not reference the plugin itself", i, d)
// 		}
// 	}
// 	for i, c := range m.Commands {
// 		if strings.TrimSpace(c.Name) == "" {
// 			return fmt.Errorf("commands[%d].name is required", i)
// 		}
// 		if strings.ContainsAny(c.Name, " \t") {
// 			return fmt.Errorf("commands[%d].name %q must not contain whitespace", i, c.Name)
// 		}
// 	}
// 	if m.UI != nil {
// 		if err := m.UI.validate(m.Name); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

// var elementTagPattern = regexp.MustCompile(`^[a-z0-9-]*$`)

// // pagePattern constrains ui.mounts page slugs (syntax only — the page
// // vocabulary itself lives in the layout, ADR-0011).
// var pagePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// // ValidComponentTag reports whether tag is a legal custom element name: what
// // customElements.define enforces (lowercase, starts with a letter, contains a
// // hyphen). Shared by Manifest validation and serve's PanelOp validation.
// func ValidComponentTag(tag string) bool {
// 	if len(tag) < 3 || tag[0] < 'a' || tag[0] > 'z' || !strings.Contains(tag, "-") {
// 		return false
// 	}
// 	return elementTagPattern.MatchString(tag)
// }

// // validate enforces the Panel Component contract (ADR-0010/0012): the UI Entry is
// // an ES Module under ui/, and every mounted component tag belongs to this
// // plugin's namespace. Slot ids may be base layout slots or contributed page slots.
// func (u *UISpec) validate(pluginName string) error {
// 	if strings.TrimSpace(u.Entry) == "" {
// 		return fmt.Errorf("ui.entry is required")
// 	}
// 	clean := path.Clean(strings.ReplaceAll(u.Entry, "\\", "/"))
// 	if strings.HasPrefix(clean, "/") || strings.Contains(clean, "..") {
// 		return fmt.Errorf("ui.entry %q must stay under ui/", u.Entry)
// 	}
// 	if !strings.HasSuffix(clean, ".js") {
// 		return fmt.Errorf("ui.entry %q must be a .js ES Module", u.Entry)
// 	}
// 	if u.Trust != "" && u.Trust != "full" && u.Trust != "isolated" {
// 		return fmt.Errorf("ui.trust must be full or isolated, got %q", u.Trust)
// 	}
// 	for i, a := range u.Assets {
// 		cleanAsset := path.Clean(strings.ReplaceAll(a, "\\", "/"))
// 		if strings.HasPrefix(cleanAsset, "/") || strings.Contains(cleanAsset, "..") || strings.TrimSpace(a) == "" {
// 			return fmt.Errorf("ui.assets[%d] %q must stay under ui/", i, a)
// 		}
// 		if cleanAsset == clean {
// 			return fmt.Errorf("ui.assets[%d] duplicates ui.entry %q", i, a)
// 		}
// 	}
// 	for i, mount := range u.Mounts {
// 		if mount.Page != "" && !pagePattern.MatchString(mount.Page) {
// 			return fmt.Errorf("ui.mounts[%d].page %q must match %v", i, mount.Page, pagePattern.String())
// 		}
// 		if !ValidMountSlot(mount.Slot) {
// 			return fmt.Errorf("ui.mounts[%d].slot %q must be a valid slot id", i, mount.Slot)
// 		}
// 		if !ValidComponentTag(mount.Component) {
// 			return fmt.Errorf("ui.mounts[%d].component %q must be a valid custom element tag", i, mount.Component)
// 		}
// 		if !strings.HasPrefix(mount.Component, pluginName+"-") {
// 			return fmt.Errorf("ui.mounts[%d].component %q must be prefixed with %q", i, mount.Component, pluginName+"-")
// 		}
// 		if len(mount.Props) > 0 {
// 			trimmed := strings.TrimSpace(string(mount.Props))
// 			if !json.Valid(mount.Props) || !strings.HasPrefix(trimmed, "{") {
// 				return fmt.Errorf("ui.mounts[%d].props must be a JSON object", i)
// 			}
// 		}
// 	}
// 	for i, p := range u.Pages {
// 		if !pagePattern.MatchString(p.Slug) {
// 			return fmt.Errorf("ui.pages[%d].slug %q must match %v", i, p.Slug, pagePattern.String())
// 		}
// 		if !strings.HasPrefix(p.Path, "/") {
// 			return fmt.Errorf("ui.pages[%d].path %q must start with /", i, p.Path)
// 		}
// 		if len(p.Slots) == 0 {
// 			return fmt.Errorf("ui.pages[%d].slots is required", i)
// 		}
// 		seen := map[string]bool{}
// 		for j, s := range p.Slots {
// 			if !ValidMountSlot(s.ID) {
// 				return fmt.Errorf("ui.pages[%d].slots[%d].id %q invalid", i, j, s.ID)
// 			}
// 			if seen[s.ID] {
// 				return fmt.Errorf("ui.pages[%d].slot %q duplicated", i, s.ID)
// 			}
// 			seen[s.ID] = true
// 		}
// 	}
// 	return nil
// }

// // UIEntryExists reports whether the UI Entry module is present under dir.
// func (m *Manifest) UIEntryExists(dir string) error {
// 	full := filepath.Join(dir, filepath.FromSlash(m.UI.NormalizedEntry()))
// 	st, err := os.Stat(full)
// 	if err != nil {
// 		return fmt.Errorf("ui.entry %q not found in %s: %w", m.UI.Entry, dir, err)
// 	}
// 	if st.IsDir() {
// 		return fmt.Errorf("ui.entry %q is a directory in %s", m.UI.Entry, dir)
// 	}
// 	return nil
// }

// // UIAssetsExist reports whether every declared ui asset file is present under dir.
// func (m *Manifest) UIAssetsExist(dir string) error {
// 	for _, a := range m.UI.Assets {
// 		clean := path.Clean(strings.ReplaceAll(a, "\\", "/"))
// 		if !strings.HasPrefix(clean, "ui/") {
// 			clean = "ui/" + clean
// 		}
// 		full := filepath.Join(dir, filepath.FromSlash(clean))
// 		st, err := os.Stat(full)
// 		if err != nil {
// 			return fmt.Errorf("ui.asset %q not found in %s: %w", a, dir, err)
// 		}
// 		if st.IsDir() {
// 			return fmt.Errorf("ui.asset %q is a directory in %s", a, dir)
// 		}
// 	}
// 	return nil
// }

// // ResolveEntry returns the absolute path of the executable under dir.
// func (m *Manifest) ResolveEntry(dir string) string {
// 	return filepath.Join(dir, m.Entry)
// }

// // EntryExists reports whether the executable file is present under dir.
// func (m *Manifest) EntryExists(dir string) error {
// 	p := m.ResolveEntry(dir)
// 	st, err := os.Stat(p)
// 	if err != nil {
// 		return fmt.Errorf("entry %q not found in %s: %w", m.Entry, dir, err)
// 	}
// 	if st.IsDir() {
// 		return fmt.Errorf("entry %q is a directory in %s", m.Entry, dir)
// 	}
// 	return nil
// }
