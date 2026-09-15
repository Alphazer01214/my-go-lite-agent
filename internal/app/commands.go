package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/plugin"
	"github.com/tomori/my-go-lite-agent/protocol"
	"github.com/tomori/my-go-lite-agent/serve"
)

// commandPlane is the Host-native slash-command router (ADR-0008).
type commandPlane struct {
	srv        *serve.Server
	pluginsDir string
	manifests  map[string]plugin.Manifest
	mounted    []string
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
			fmt.Fprintf(os.Stderr, "refresh: skip %s (conflicts with native command)\n", m.Name)
			continue
		}
		if _, mounted := cp.manifests[m.Name]; mounted {
			next[m.Name] = m
		}
	}
	for name := range cp.manifests {
		if m, ok := next[name]; ok {
			cp.manifests[name] = m
		}
	}
	// Broadcast config.reload so running plugins re-read disk config (ADR / plugin-config).
	// Plugins without the method fail the Frame; that is expected and ignored.
	for _, name := range cp.mounted {
		out, err := cp.srv.Call(name, &protocol.Frame{
			V:       protocol.Version,
			Type:    protocol.TypeReq,
			Cap:     "config",
			Method:  "reload",
			Payload: json.RawMessage(`{}`),
		})
		if err != nil || (out != nil && out.Error != nil) {
			continue
		}
	}
	return nil
}

// handle runs a slash line for the CLI (prints to stdout).
func (cp *commandPlane) handle(line string) (quit bool, err error) {
	out, quit, err := cp.handleOut(line)
	if out != "" {
		fmt.Print(out)
		if !strings.HasSuffix(out, "\n") {
			fmt.Println()
		}
	}
	return quit, err
}

// handleOut runs a slash line and returns printable output (Web Shell uses this).
func (cp *commandPlane) handleOut(line string) (output string, quit bool, err error) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '/' {
		return "", false, fmt.Errorf("not a command: %q", line)
	}
	body := strings.TrimSpace(line[1:])
	if body == "" {
		return cp.helpText(""), false, nil
	}
	fields := strings.Fields(body)
	name := strings.ToLower(fields[0])
	rest := strings.TrimSpace(strings.TrimPrefix(body, fields[0]))

	switch name {
	case "exit":
		return "", true, nil
	case "help":
		return cp.helpText(rest), false, nil
	case "lp":
		return cp.pluginsText(), false, nil
	case "refresh":
		if err := cp.refresh(); err != nil {
			return "", false, err
		}
		return "refresh ok — manifests updated, config.reload broadcast (no hot-plug)\n", false, nil
	}

	if _, ok := cp.manifests[name]; ok {
		sub, args := splitCommandRest(rest)
		if sub == "" {
			return cp.helpText(name), false, nil
		}
		payload, err := cp.srv.CallCommand(name, sub, args)
		if err != nil {
			return "", false, err
		}
		return commandResultText(payload), false, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "unknown command: %s\n", name)
	if sug := suggestCommand(name, cp.nativeAndPluginNames()); len(sug) > 0 {
		fmt.Fprintf(&b, "did you mean: %s\n", strings.Join(sug, ", "))
	}
	return b.String(), false, nil
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

func commandResultText(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(payload, &out); err == nil && out.Text != "" {
		return strings.TrimRight(out.Text, "\n") + "\n"
	}
	return string(payload) + "\n"
}

func (cp *commandPlane) helpText(pluginName string) string {
	pluginName = strings.ToLower(strings.TrimSpace(pluginName))
	var b strings.Builder
	if pluginName != "" {
		m, ok := cp.manifests[pluginName]
		if !ok {
			fmt.Fprintf(&b, "unknown plugin: %s\n", pluginName)
			return b.String()
		}
		fmt.Fprintf(&b, "%s  v%s  %s\n", m.Name, m.Version, m.Description)
		if len(m.Commands) == 0 {
			b.WriteString("  (no commands)\n")
			return b.String()
		}
		for _, c := range m.Commands {
			usage := c.Usage
			if usage == "" {
				usage = "/" + m.Name + " " + c.Name
			}
			fmt.Fprintf(&b, "  %s\n    %s\n", usage, c.Description)
		}
		return b.String()
	}

	b.WriteString("Native commands:\n")
	b.WriteString("  /help [plugin]     Show this help, or a plugin's commands\n")
	b.WriteString("  /lp                List mounted plugins (name, version, provides)\n")
	b.WriteString("  /refresh           Rescan manifests and broadcast config.reload (no hot-plug)\n")
	b.WriteString("  /exit              Quit (CLI only)\n")
	b.WriteString("\nPlugin commands:\n")
	b.WriteString("  Use /<plugin> to list its commands, or /<plugin> <cmd> [args].\n")
	b.WriteString("  Session Log ops live on the session plugin, e.g. /session dump-trace\n")
	b.WriteString("  Context Manager: /context-manager usage | list (Model Context) | skills\n")
	b.WriteString("\nCLI flags (one-shot, not slash):\n")
	b.WriteString("  -turn TEXT           Run one default-Loop turn\n")
	b.WriteString("  -context-list N      Print last N prepare messages after the run\n")
	b.WriteString("  -session-derive      Print Model Context from session.derive\n")
	b.WriteString("  -session-query       Print Session Log facts\n")
	b.WriteString("  -session-append JSON Append facts\n")
	b.WriteString("  -invoke / -frame-cap / -frame-method / -invoke-payload\n")
	b.WriteString("                       Host-initiated Frame call into a plugin\n")
	b.WriteString("\nPlugins:\n")
	if len(cp.mounted) == 0 {
		b.WriteString("  (none)\n")
		return b.String()
	}
	for _, name := range cp.mounted {
		m := cp.manifests[name]
		fmt.Fprintf(&b, "  %s  v%s  %s\n", m.Name, m.Version, m.Description)
		for _, c := range m.Commands {
			usage := c.Usage
			if usage == "" {
				usage = "/" + m.Name + " " + c.Name
			}
			fmt.Fprintf(&b, "    %s\n", usage)
		}
	}
	return b.String()
}

func (cp *commandPlane) pluginsText() string {
	var b strings.Builder
	if len(cp.mounted) == 0 {
		b.WriteString("(no plugins mounted)\n")
		return b.String()
	}
	for _, name := range cp.mounted {
		m := cp.manifests[name]
		fmt.Fprintf(&b, "%s\tv%s\tprovides=[%s]\t%s\n",
			m.Name, m.Version, strings.Join(m.Provides, ","), m.Description)
	}
	return b.String()
}

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
