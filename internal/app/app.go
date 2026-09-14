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

func runAssembly(pluginsDir, assemblyPath string, dump bool) error {
	cfg, err := assembly.Load(assemblyPath)
	if err != nil {
		return err
	}
	res := discovery.Scan(pluginsDir)
	if len(res.Errors) > 0 {
		printDiscovery(res)
		return fmt.Errorf("discovery failed before assembly")
	}
	plan := assembly.Resolve(cfg, res)
	printRejected(plan.Rejected)
	if len(plan.Missing) > 0 {
		return fmt.Errorf("assembly references unknown plugins: %s", strings.Join(plan.Missing, ", "))
	}

	for _, p := range plan.Mounted {
		if err := probePlugin(p); err != nil {
			return fmt.Errorf("mount %s: %w", p.Manifest.Name, err)
		}
	}

	if dump {
		dumpAssembly(plan)
	}
	return nil
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

// probePlugin starts the Plugin, completes one echo Frame, then shuts it down.
// UI-only Plugins have no executable to probe (ADR-0011).
func probePlugin(p discovery.Found) error {
	fmt.Printf("mount name=%s\n", p.Manifest.Name)
	if p.Manifest.Entry == "" {
		return nil
	}
	return roundtrip(p.Manifest.ResolveEntry(p.Dir), p.Manifest.Name)
}

func runEchoRoundtrip(pluginPath string) error {
	return roundtrip(pluginPath, "1")
}

func roundtrip(pluginPath, id string) error {
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
		Cap:     "echo",
		Method:  "echo",
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

func runCallPlugin(pluginsDir, assemblyPath, name string, dump bool) error {
	cfg, err := assembly.Load(assemblyPath)
	if err != nil {
		return err
	}
	res := discovery.Scan(pluginsDir)
	if len(res.Errors) > 0 {
		printDiscovery(res)
		return fmt.Errorf("discovery failed before assembly")
	}
	plan := assembly.Resolve(cfg, res)
	printRejected(plan.Rejected)
	if len(plan.Missing) > 0 {
		return fmt.Errorf("assembly references unknown plugins: %s", strings.Join(plan.Missing, ", "))
	}
	if dump {
		dumpAssembly(plan)
	}
	srv, err := serve.Start(plan.Mounted)
	if err != nil {
		return err
	}
	defer func() { _ = srv.Close() }()

	frame := &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     name,
		Method:  "echo",
		Payload: json.RawMessage(`{"hello":"lifecycle"}`),
	}
	out, err := srv.Call(name, frame)
	if err != nil {
		return err
	}
	if out.Error != nil {
		return fmt.Errorf("call failed: %s: %s", out.Error.Code, out.Error.Message)
	}
	fmt.Printf("ok plugin=%s payload=%s\n", name, string(out.Payload))
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
		_, err := srv.CallCommand(p.Manifest.Name, "__probe__", "")
		if err == nil {
			continue
		}
		if strings.Contains(err.Error(), "method_not_found") {
			fmt.Fprintf(os.Stderr, "warn: plugin %s declares commands but has no commands.call handler\n", p.Manifest.Name)
		}
	}
}
