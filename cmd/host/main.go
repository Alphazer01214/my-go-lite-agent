// Command host is the thin kernel entry.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/protocol"
)

func main() {
	pluginPath := flag.String("plugin", "", "path to plugin executable (single Frame round-trip)")
	discoverDir := flag.String("discover", "", "scan plugin directory and list discovered plugins (no mount)")
	pluginsDir := flag.String("plugins", "", "plugin directory for assembly")
	assemblyPath := flag.String("assembly", "", "assembly config path (JSON plugins list)")
	dump := flag.Bool("dump", false, "dump assembly tree after resolve")
	flag.Parse()

	switch {
	case *discoverDir != "":
		if err := runDiscover(*discoverDir); err != nil {
			fatal(err)
		}
	case *assemblyPath != "":
		if *pluginsDir == "" {
			fatal(fmt.Errorf("-assembly requires -plugins"))
		}
		if err := runAssembly(*pluginsDir, *assemblyPath, *dump); err != nil {
			fatal(err)
		}
	case *pluginPath != "":
		if err := runEchoRoundtrip(*pluginPath); err != nil {
			fatal(err)
		}
	default:
		fmt.Fprintln(os.Stderr, "host: -plugin, -discover, or -assembly is required")
		os.Exit(2)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "host: %v\n", err)
	os.Exit(1)
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
	if len(plan.Missing) > 0 {
		return fmt.Errorf("assembly references unknown plugins: %s", strings.Join(plan.Missing, ", "))
	}

	for _, p := range plan.Mounted {
		if err := probePlugin(p); err != nil {
			return fmt.Errorf("mount %s: %w", p.Manifest.Name, err)
		}
	}

	if dump {
		dumpAssembly(plan, res)
	}
	return nil
}

func dumpAssembly(plan assembly.Plan, res discovery.Result) {
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
	_ = res
}

// probePlugin starts the Plugin, completes one echo Frame, then shuts it down.
func probePlugin(p discovery.Found) error {
	fmt.Printf("mount name=%s\n", p.Manifest.Name)
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
		V:       1,
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
