// Package layout is the Web Medium's page/slot declaration (ADR-0012).
// Disk layout.json is the base; plugins contribute additively via Manifest.
package layout

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Trust is the reserved isolation flag (ADR-0012): only "full" is implemented.
type Trust string

const (
	TrustFull    Trust = "full"
	TrustIsolated Trust = "isolated"
)

// Slot is one Panel slot on a page.
type Slot struct {
	ID       string `json:"id"`
	Role     string `json:"role,omitempty"`
	Preferred string `json:"preferred,omitempty"`
	Region   string `json:"region,omitempty"`
}

// Page is one layout page (path chrome + slots).
type Page struct {
	Slug  string `json:"slug"`
	Title string `json:"title,omitempty"`
	Path  string `json:"path"`
	Slots []Slot `json:"slots"`
}

// Doc is the on-disk layout.json shape.
type Doc struct {
	Trust  Trust  `json:"trust,omitempty"`
	Pages  []Page `json:"pages"`
}

// Merged is the result after plugin contributions and Assembly overlays.
type Merged struct {
	Trust Trust  `json:"trust,omitempty"`
	Pages []Page `json:"pages"`
}

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Load reads and validates a layout.json file. Missing file is an error (ADR-0012).
func Load(path string) (Doc, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Doc{}, fmt.Errorf("read layout %s: %w", path, err)
	}
	var d Doc
	if err := json.Unmarshal(raw, &d); err != nil {
		return Doc{}, fmt.Errorf("parse layout %s: %w", path, err)
	}
	if err := d.Validate(); err != nil {
		return Doc{}, fmt.Errorf("invalid layout %s: %w", path, err)
	}
	return d, nil
}

// Validate checks required pages/slots and reserved main page.
func (d Doc) Validate() error {
	if len(d.Pages) == 0 {
		return fmt.Errorf("layout.pages is required")
	}
	if d.Trust != "" && d.Trust != TrustFull && d.Trust != TrustIsolated {
		return fmt.Errorf("layout.trust must be full or isolated, got %q", d.Trust)
	}
	hasMain := false
	seenPage := map[string]bool{}
	for i, p := range d.Pages {
		if !slugPattern.MatchString(p.Slug) {
			return fmt.Errorf("pages[%d].slug %q must match %v", i, p.Slug, slugPattern.String())
		}
		if seenPage[p.Slug] {
			return fmt.Errorf("pages[%d].slug %q duplicated", i, p.Slug)
		}
		seenPage[p.Slug] = true
		if p.Slug == "main" {
			hasMain = true
		}
		if !strings.HasPrefix(p.Path, "/") {
			return fmt.Errorf("pages[%d].path %q must start with /", i, p.Path)
		}
		if len(p.Slots) == 0 {
			return fmt.Errorf("pages[%d] (%s).slots is required", i, p.Slug)
		}
		seenSlot := map[string]bool{}
		for j, s := range p.Slots {
			if strings.TrimSpace(s.ID) == "" {
				return fmt.Errorf("pages[%d].slots[%d].id is required", i, j)
			}
			if seenSlot[s.ID] {
				return fmt.Errorf("pages[%d].slot %q duplicated", i, s.ID)
			}
			seenSlot[s.ID] = true
		}
	}
	if !hasMain {
		return fmt.Errorf("layout must define page \"main\"")
	}
	return nil
}

// Contribution is a plugin's additive ui.pages (Manifest, ADR-0012).
type Contribution struct {
	Plugin string
	Pages  []Page
}

// Merge applies additive plugin contributions onto the base layout.
// Plugins may add pages; they may not replace or delete base pages/slots.
func Merge(base Doc, contribs []Contribution) (Merged, error) {
	m := Merged{Trust: base.Trust}
	if m.Trust == "" {
		m.Trust = TrustFull
	}
	bySlug := map[string]int{}
	for i, p := range base.Pages {
		m.Pages = append(m.Pages, clonePage(p))
		bySlug[p.Slug] = i
	}
	for _, c := range contribs {
		for _, p := range c.Pages {
			if !slugPattern.MatchString(p.Slug) {
				return Merged{}, fmt.Errorf("plugin %s page %q: invalid slug", c.Plugin, p.Slug)
			}
			if idx, ok := bySlug[p.Slug]; ok {
				// Additive only: append unknown slots to the existing page.
				existing := map[string]bool{}
				for _, s := range m.Pages[idx].Slots {
					existing[s.ID] = true
				}
				for _, s := range p.Slots {
					if existing[s.ID] {
						return Merged{}, fmt.Errorf("plugin %s may not redefine slot %q on page %q", c.Plugin, s.ID, p.Slug)
					}
					if strings.TrimSpace(s.ID) == "" {
						return Merged{}, fmt.Errorf("plugin %s page %q: empty slot id", c.Plugin, p.Slug)
					}
					m.Pages[idx].Slots = append(m.Pages[idx].Slots, s)
					existing[s.ID] = true
				}
				if p.Title != "" && m.Pages[idx].Title == "" {
					m.Pages[idx].Title = p.Title
				}
				continue
			}
			if err := validateContribPage(p); err != nil {
				return Merged{}, fmt.Errorf("plugin %s: %w", c.Plugin, err)
			}
			m.Pages = append(m.Pages, clonePage(p))
			bySlug[p.Slug] = len(m.Pages) - 1
		}
		// Standalone slots without a page target are ignored in v2 — they must
		// ride a page contribution.
	}
	return m, nil
}

func validateContribPage(p Page) error {
	if !strings.HasPrefix(p.Path, "/") {
		return fmt.Errorf("page %q path must start with /", p.Slug)
	}
	if len(p.Slots) == 0 {
		return fmt.Errorf("page %q needs at least one slot", p.Slug)
	}
	seen := map[string]bool{}
	for _, s := range p.Slots {
		if strings.TrimSpace(s.ID) == "" {
			return fmt.Errorf("page %q: empty slot id", p.Slug)
		}
		if seen[s.ID] {
			return fmt.Errorf("page %q: duplicate slot %q", p.Slug, s.ID)
		}
		seen[s.ID] = true
	}
	return nil
}

func clonePage(p Page) Page {
	out := Page{Slug: p.Slug, Title: p.Title, Path: p.Path}
	out.Slots = append([]Slot(nil), p.Slots...)
	return out
}
