package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/plugin"
	"github.com/tomori/my-go-lite-agent/serve"
)

// commandPlane is the Host-native slash-command router (ADR-0008).
type commandPlane struct {
	srv        *serve.Server
	pluginsDir string
	// manifests is the in-memory Manifest cache (/refresh updates it; no hot-plug).
	manifests map[string]plugin.Manifest
	// mounted names in assembly order
	mounted []string
}

func newCommandPlane(srv *serve.Server, pluginsDir string, plan assembly.Plan) *commandPlane {
	cp := &commandPlane{
		srv:        srv,
		pluginsDir: pluginsDir,
		manifests:  map[string]plugin.Manifest{},
	}
	for _, p := range plan.Mounted {
		cp.manifests[p.Manifest.Name] = p.Manifest
		cp.mounted = append(cp.mounted, p.Manifest.Name)
	}
	return cp
}

func (cp *commandPlane) refresh() error {
	res := discovery.Scan(cp.pluginsDir)
	if len(res.Errors) > 0 {
		var b strings.Builder
		for _, e := range res.Errors {
			fmt.Fprintf(&b, "  %v\n", e)
		}
		return fmt.Errorf("refresh scan errors:\n%s", b.String())
	}
	next := map[string]plugin.Manifest{}
	for _, p := range res.Plugins {
		m := p.Manifest
		if m.ConflictsWithNativeCommand() {
			fmt.Fprintf(os.Stderr, "refresh: skip %s (%s)\n", m.Name, "conflicts with native command")
			continue
		}
		// Only update metadata for already-mounted plugins; new ones are not hot-plugged.
		if _, mounted := cp.manifests[m.Name]; mounted {
			next[m.Name] = m
		}
	}
	// Keep mounted set; replace manifests for those still present.
	for name := range cp.manifests {
		if m, ok := next[name]; ok {
			cp.manifests[name] = m
		}
	}
	return nil
}

// handle processes one slash line. Returns quit=true when the user asked to exit.
func (cp *commandPlane) handle(line string) (quit bool, err error) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '/' {
		return false, fmt.Errorf("not a command: %q", line)
	}
	body := strings.TrimSpace(line[1:])
	if body == "" {
		cp.printHelp("")
		return false, nil
	}
	fields := strings.Fields(body)
	name := strings.ToLower(fields[0])
	rest := strings.TrimSpace(strings.TrimPrefix(body, fields[0]))

	switch name {
	case "exit":
		return true, nil
	case "help":
		cp.printHelp(rest)
		return false, nil
	case "lp":
		cp.printPlugins()
		return false, nil
	case "refresh":
		if err := cp.refresh(); err != nil {
			return false, err
		}
		fmt.Println("refresh ok — manifests updated (no hot-plug)")
		return false, nil
	}

	// Plugin command: /plugin [subcmd] [args...]
	if _, ok := cp.manifests[name]; ok {
		sub, args := splitCommandRest(rest)
		if sub == "" {
			cp.printHelp(name)
			return false, nil
		}
		payload, err := cp.srv.CallCommand(name, sub, args)
		if err != nil {
			return false, err
		}
		cp.printCommandResult(payload)
		return false, nil
	}

	fmt.Printf("unknown command: %s\n", name)
	if sug := suggestCommand(name, cp.nativeAndPluginNames()); len(sug) > 0 {
		fmt.Printf("did you mean: %s\n", strings.Join(sug, ", "))
	}
	return false, nil
}

func splitCommandRest(rest string) (sub, args string) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", ""
	}
	parts := strings.SplitN(rest, " ", 2)
	sub = parts[0]
	if len(parts) == 2 {
		args = strings.TrimSpace(parts[1])
	}
	return sub, args
}

func (cp *commandPlane) nativeAndPluginNames() []string {
	names := []string{"help", "lp", "refresh", "exit"}
	names = append(names, cp.mounted...)
	return names
}

func (cp *commandPlane) printCommandResult(payload json.RawMessage) {
	if len(payload) == 0 {
		return
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(payload, &out); err == nil && out.Text != "" {
		fmt.Println(out.Text)
		return
	}
	fmt.Println(string(payload))
}

func (cp *commandPlane) printHelp(pluginName string) {
	pluginName = strings.ToLower(strings.TrimSpace(pluginName))
	if pluginName != "" {
		m, ok := cp.manifests[pluginName]
		if !ok {
			fmt.Printf("unknown plugin: %s\n", pluginName)
			return
		}
		fmt.Printf("%s  v%s  %s\n", m.Name, m.Version, m.Description)
		if len(m.Commands) == 0 {
			fmt.Println("  (no commands)")
			return
		}
		for _, c := range m.Commands {
			usage := c.Usage
			if usage == "" {
				usage = "/" + m.Name + " " + c.Name
			}
			fmt.Printf("  %s\n    %s\n", usage, c.Description)
		}
		return
	}

	fmt.Println("Native commands:")
	fmt.Println("  /help [plugin]     Show this help, or a plugin's commands")
	fmt.Println("  /lp                List mounted plugins")
	fmt.Println("  /refresh           Rescan plugin directory (metadata only)")
	fmt.Println("  /exit              Quit")
	fmt.Println()
	fmt.Println("Plugins:")
	if len(cp.mounted) == 0 {
		fmt.Println("  (none)")
		return
	}
	for _, name := range cp.mounted {
		m := cp.manifests[name]
		fmt.Printf("  %s  v%s  %s\n", m.Name, m.Version, m.Description)
		if len(m.Commands) == 0 {
			continue
		}
		for _, c := range m.Commands {
			usage := c.Usage
			if usage == "" {
				usage = "/" + m.Name + " " + c.Name
			}
			fmt.Printf("    %s\n", usage)
		}
	}
}

func (cp *commandPlane) printPlugins() {
	if len(cp.mounted) == 0 {
		fmt.Println("(no plugins mounted)")
		return
	}
	for _, name := range cp.mounted {
		m := cp.manifests[name]
		fmt.Printf("%s\tv%s\tprovides=[%s]\t%s\n",
			m.Name, m.Version, strings.Join(m.Provides, ","), m.Description)
	}
}

// suggestCommand returns names within edit distance 2 of input.
func suggestCommand(input string, candidates []string) []string {
	var out []string
	for _, c := range candidates {
		if editDistance(input, c) <= 2 {
			out = append(out, "/"+c)
		}
	}
	sort.Strings(out)
	return out
}

func editDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	la, lb := len(ar), len(br)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[lb]
}
