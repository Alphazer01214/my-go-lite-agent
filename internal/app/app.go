// Package app hosts the runtime shared by the two entries: flag dispatch,
// assembly mounting helpers, the CLI renderer, the REPL loop, the command
// plane, and the Web Medium launcher. liteagent-cli and liteagent-server are
// thin mains over it (ADR-0011: divergence is frontend-only).
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/protocol"
	"github.com/tomori/my-go-lite-agent/serve"
)

var progName = "liteagent"

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", progName, err)
	os.Exit(1)
}

func printCards(cards []serve.PresentationCard) {
	for i, c := range cards {
		fmt.Printf("card[%d] type=%s tool=%s data=%s\n", i, c.CardType, c.Tool, string(c.Data))
	}
}

func runDiscover(dir string) error {
	res := discovery.Scan(dir)
	printDiscovery(res)
	if len(res.Errors) > 0 {
		return fmt.Errorf("discovery failed for %d director%s", len(res.Errors), plural(len(res.Errors)))
	}
	return nil
}

func printDiscovery(res discovery.Result) {
	for _, p := range res.Plugins {
		m := p.Manifest
		fmt.Printf("%s\t%s\tprotocol=%d\tprovides=[%s]\tconsumes=[%s]\tentry=%s\n",
			m.Name, m.Version, m.Protocol,
			strings.Join(m.Provides, ","),
			strings.Join(m.Consumes, ","),
			m.Entry,
		)
	}
	for _, e := range res.Errors {
		fmt.Fprintf(os.Stderr, "discover error: %v\n", e)
	}
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// resolveAssembly scans plugins and builds the mount plan (ADR-0021).
// The daily path is Autostart roots + dependsOn closure; -assembly is a
// deprecated no-op (its file is ignored — assembly files no longer act as a
// mount whitelist). Soft-fail (ADR-0017/0022): discovery errors print but do
// not abort.
func resolveAssembly(pluginsDir, assemblyPath string, dump bool) (assembly.Plan, assembly.Config, error) {
	if assemblyPath != "" {
		fmt.Fprintf(os.Stderr, "warn: -assembly is deprecated (ADR-0021) and ignored; Autostart+dependsOn is used (file: %s)\n", assemblyPath)
	}
	res := discovery.Scan(pluginsDir)
	for _, e := range res.Errors {
		fmt.Fprintf(os.Stderr, "discover error: %v\n", e)
	}
	plan := assembly.ResolveAutostart(res)
	printRejected(plan.Rejected)
	if len(plan.Missing) > 0 {
		fmt.Fprintf(os.Stderr, "assembly references unknown plugins (continuing): %s\n", strings.Join(plan.Missing, ", "))
	}
	if dump {
		dumpAssembly(plan)
	}
	return plan, assembly.Config{}, nil
}

func runAssembly(pluginsDir, assemblyPath string, dump bool, capName, method string) error {
	plan, _, err := resolveAssembly(pluginsDir, assemblyPath, dump)
	if err != nil {
		return err
	}

	for _, p := range plan.Mounted {
		if err := probePlugin(p, capName, method); err != nil {
			return fmt.Errorf("mount %s: %w", p.Manifest.Name, err)
		}
	}
	return nil
}

// startMounted starts the Server for plan and attaches the Discovery catalog
// so ensurePlugins can mount later (ADR-0023).
func startMounted(pluginsDir string, plan assembly.Plan) (*serve.Server, error) {
	res := discovery.Scan(pluginsDir)
	srv, err := serve.Start(plan.Mounted)
	if err != nil {
		return nil, err
	}
	srv.SetCatalog(res)
	srv.SetPluginsDir(pluginsDir)
	return srv, nil
}

func dumpAssembly(plan assembly.Plan) {
	fmt.Println("assembly:")
	for _, p := range plan.Mounted {
		m := p.Manifest
		fmt.Printf("  mounted name=%s dir=%s entry=%s provides=[%s]\n",
			m.Name, p.Dir, m.Entry, strings.Join(m.Provides, ","))
	}
	for _, p := range plan.Unmounted {
		m := p.Manifest
		fmt.Printf("  available name=%s mounted=false\n", m.Name)
	}
}

// probePlugin starts the Plugin, completes one Frame, then shuts it down.
// UI-only Plugins have no executable to probe (ADR-0011). The probe capability
// and method come from the caller (-frame-cap/-frame-method, ADR-0026).
func probePlugin(p discovery.Found, capName, method string) error {
	fmt.Printf("mount name=%s\n", p.Manifest.Name)
	if p.Manifest.Entry == "" {
		return nil
	}
	return roundtrip(p.Manifest.ResolveEntry(p.Dir), p.Manifest.Name, capName, method)
}

// runPluginRoundtrip sends one Frame round-trip to a plugin executable.
func runPluginRoundtrip(pluginPath, capName, method string) error {
	return roundtrip(pluginPath, "1", capName, method)
}

// roundtrip sends one Frame to a plugin executable and waits for its res.
// cap/method come from the caller (-frame-cap/-frame-method): the probe no
// longer assumes a plugin name (ADR-0026).
func roundtrip(pluginPath, id, capName, method string) error {
	cmd := exec.Command(pluginPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start plugin: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	req := &protocol.Frame{
		V:       protocol.Version,
		ID:      id,
		Type:    protocol.TypeReq,
		Cap:     capName,
		Method:  method,
		Payload: json.RawMessage(`{"hello":"world"}`),
	}
	if err := protocol.WriteFrame(stdin, req); err != nil {
		return fmt.Errorf("write req: %w", err)
	}
	res, err := protocol.ReadFrame(stdout)
	if err != nil {
		return fmt.Errorf("read res: %w", err)
	}
	if res.Type != protocol.TypeRes || res.ID != req.ID {
		return fmt.Errorf("unexpected frame: type=%s id=%s", res.Type, res.ID)
	}
	if res.Error != nil {
		return fmt.Errorf("plugin error: %w", res.Error)
	}
	fmt.Printf("ok id=%s payload=%s\n", res.ID, string(res.Payload))
	return nil
}

func runCallPlugin(pluginsDir, assemblyPath, name string, dump bool, method string) error {
	plan, _, err := resolveAssembly(pluginsDir, assemblyPath, dump)
	if err != nil {
		return err
	}
	srv, err := startMounted(pluginsDir, plan)
	if err != nil {
		return err
	}
	defer func() { _ = srv.Close() }()

	// -call-plugin is a diagnostic surface: address the plugin by name (L0).
	payload, err := srv.CallByPlugin(name, name, method, json.RawMessage(`{"hello":"lifecycle"}`))
	if err != nil {
		return err
	}
	fmt.Printf("ok plugin=%s payload=%s\n", name, string(payload))
	return nil
}

func printRejected(rejected []assembly.Rejected) {
	for _, r := range rejected {
		fmt.Fprintf(os.Stderr, "reject plugin %s: %s\n", r.Name, r.Reason)
	}
}

// probeCommandFaces warns when a Plugin declares commands[] but has no commands.call handler.
func probeCommandFaces(srv *serve.Server, mounted []discovery.Found) {
	for _, p := range mounted {
		if len(p.Manifest.Commands) == 0 {
			continue
		}
		_, err := serve.CallByFace(srv, p.Manifest.Name, "commands", "call", json.RawMessage(`{"command":"__probe__","args":""}`))
		if err == nil {
			continue
		}
		if strings.Contains(err.Error(), "method_not_found") {
			fmt.Fprintf(os.Stderr, "warn: plugin %s declares commands but has no commands.call handler\n", p.Manifest.Name)
		}
	}
}
