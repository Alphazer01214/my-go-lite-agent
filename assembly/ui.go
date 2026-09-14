package assembly

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tomori/my-go-lite-agent/plugin"
)

// EffectiveMount is one mount after Assembly UI adjudication (ADR-0012).
type EffectiveMount struct {
	Plugin    string          `json:"plugin"`
	Page      string          `json:"page"`
	Slot      string          `json:"slot"`
	Component string          `json:"component"`
	Props     json.RawMessage `json:"props,omitempty"`
	Winner    bool            `json:"winner,omitempty"`
	Order     int             `json:"order,omitempty"`
}

// ResolveUIMounts applies Assembly.ui over every mounted plugin's Manifest mounts.
func ResolveUIMounts(cfg Config, plan Plan) ([]EffectiveMount, error) {
	var out []EffectiveMount
	disabled := map[string]bool{}
	var overrides []UIOverride
	if cfg.UI != nil {
		for _, d := range cfg.UI.Disable {
			disabled[strings.TrimSpace(d)] = true
		}
		overrides = cfg.UI.Overrides
	}

	type key struct {
		plugin, component string
	}
	ovByKey := map[key]UIOverride{}
	for _, o := range overrides {
		if o.Plugin == "" || o.Component == "" {
			return nil, fmt.Errorf("ui.override requires plugin and component")
		}
		ovByKey[key{o.Plugin, o.Component}] = o
	}

	for _, p := range plan.Mounted {
		m := p.Manifest
		if m.UI == nil {
			continue
		}
		for _, mount := range m.UI.Mounts {
			page := mount.Page
			if page == "" {
				page = "main"
			}
			if disabled[m.Name+"/"+mount.Component] ||
				disabled[m.Name+"/"+page+"/"+mount.Slot+"/"+mount.Component] {
				continue
			}
			em := EffectiveMount{
				Plugin:    m.Name,
				Page:      page,
				Slot:      mount.Slot,
				Component: mount.Component,
				Props:     mount.Props,
			}
			if o, ok := ovByKey[key{m.Name, mount.Component}]; ok {
				if o.Page != "" {
					em.Page = o.Page
				}
				if o.Slot != "" {
					em.Slot = o.Slot
				}
				if len(o.Props) > 0 {
					em.Props = o.Props
				}
				em.Winner = o.Winner
				if o.Order != nil {
					em.Order = *o.Order
				}
			}
			out = append(out, em)
		}
	}

	// Winner: if any mount on (page,slot) is winner, keep only winners there.
	type slotKey struct{ page, slot string }
	winners := map[slotKey]bool{}
	for _, em := range out {
		if em.Winner {
			winners[slotKey{em.Page, em.Slot}] = true
		}
	}
	filtered := out[:0]
	for _, em := range out {
		if winners[slotKey{em.Page, em.Slot}] && !em.Winner {
			continue
		}
		filtered = append(filtered, em)
	}
	out = filtered

	// Sort by explicit Order first; ties keep a stable component-name order.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Page != out[j].Page {
			return out[i].Page < out[j].Page
		}
		if out[i].Slot != out[j].Slot {
			return out[i].Slot < out[j].Slot
		}
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Component < out[j].Component
	})
	return out, nil
}

// ManifestUIContributions collects additive ui.pages from mounted plugins.
func ManifestUIContributions(plan Plan) []struct {
	Plugin string
	Pages  []plugin.UIPage
} {
	var out []struct {
		Plugin string
		Pages  []plugin.UIPage
	}
	for _, p := range plan.Mounted {
		if p.Manifest.UI == nil || len(p.Manifest.UI.Pages) == 0 {
			continue
		}
		out = append(out, struct {
			Plugin string
			Pages  []plugin.UIPage
		}{Plugin: p.Manifest.Name, Pages: p.Manifest.UI.Pages})
	}
	return out
}
