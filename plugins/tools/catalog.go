package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type infoResult struct {
	tools      []catalogTool
	diagnostic *diagnostic
}

func (p *plugin) ensureCatalog(ctx context.Context, force bool) (*catalog, error) {
	p.mu.Lock()
	if !force && p.catalog != nil && time.Since(p.catalog.BuiltAt) < p.cfg.refreshTTL() {
		current := p.catalog
		p.mu.Unlock()
		return current, nil
	}
	if p.refreshing {
		done := p.refreshDone
		p.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		p.mu.Lock()
		current := p.catalog
		p.mu.Unlock()
		if current == nil {
			return nil, errors.New("tool catalog is unavailable")
		}
		return current, nil
	}
	p.refreshing = true
	done := make(chan struct{})
	p.refreshDone = done
	p.mu.Unlock()

	built, err := p.refreshCatalog(ctx)

	p.mu.Lock()
	if built != nil {
		p.catalog = built
	}
	p.refreshing = false
	close(done)
	current := p.catalog
	p.mu.Unlock()

	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, errors.New("tool catalog is unavailable")
	}
	return current, nil
}

func (p *plugin) refreshCatalog(ctx context.Context) (*catalog, error) {
	capabilities := p.cfg.Capabilities
	results := make([]infoResult, len(capabilities))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, maxInfoParallel)

	for index, capability := range capabilities {
		wg.Add(1)
		go func(index int, capability string) {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				results[index] = infoResult{diagnostic: &diagnostic{
					Capability: capability,
					Code:       "info_cancelled",
					Message:    ctx.Err().Error(),
				}}
				return
			}

			callCtx, cancel := context.WithTimeout(ctx, p.cfg.infoTimeout())
			defer cancel()
			raw, err := p.sdk.Call(callCtx, capability, "info", json.RawMessage(`{}`))
			if err != nil {
				results[index] = infoResult{diagnostic: infoDiagnostic(capability, err)}
				return
			}
			tools, diagnostic := validateProviderInfo(capability, raw)
			results[index] = infoResult{tools: tools, diagnostic: diagnostic}
		}(index, capability)
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return buildCatalog(p.seq.Add(1), results, p.configDiagnostics), nil
}

func buildCatalog(generation int64, results []infoResult, configDiagnostics []diagnostic) *catalog {
	diagnostics := append([]diagnostic(nil), configDiagnostics...)
	byName := make(map[string][]catalogTool)

	for _, result := range results {
		if result.diagnostic != nil {
			diagnostics = append(diagnostics, *result.diagnostic)
			continue
		}
		for _, tool := range result.tools {
			byName[tool.Name] = append(byName[tool.Name], tool)
		}
	}

	tools := make([]catalogTool, 0, len(byName))
	for name, candidates := range byName {
		if len(candidates) > 1 {
			diagnostics = append(diagnostics, diagnostic{
				Code:    codeToolNameConflict,
				Message: conflictMessage(name, candidates),
			})
			continue
		}
		tools = append(tools, candidates[0])
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	if len(tools) > maxTools {
		diagnostics = append(diagnostics, diagnostic{
			Code:    "too_many_tools",
			Message: fmt.Sprintf("catalog truncated to %d tools", maxTools),
		})
		tools = tools[:maxTools]
	}

	index := make(map[string]catalogTool, len(tools))
	for _, tool := range tools {
		index[tool.Name] = tool
	}

	sort.Slice(diagnostics, func(i, j int) bool {
		if diagnostics[i].Capability != diagnostics[j].Capability {
			return diagnostics[i].Capability < diagnostics[j].Capability
		}
		if diagnostics[i].Code != diagnostics[j].Code {
			return diagnostics[i].Code < diagnostics[j].Code
		}
		return diagnostics[i].Message < diagnostics[j].Message
	})

	return &catalog{
		Generation:  generation,
		BuiltAt:     time.Now(),
		Tools:       tools,
		ByName:      index,
		Diagnostics: diagnostics,
	}
}

func infoDiagnostic(capability string, err error) *diagnostic {
	code := "info_failed"
	switch pluginsdk.Code(err) {
	case pluginsdk.CodeCallDeadlineExceeded:
		code = "info_timeout"
	case pluginsdk.CodeMethodNotFound:
		code = "info_method_missing"
	case pluginsdk.CodeCallCancelled:
		code = "info_cancelled"
	}
	return &diagnostic{
		Capability: capability,
		Code:       code,
		Message:    err.Error(),
	}
}

func ctxError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return pluginsdk.ErrCode(pluginsdk.CodeCallDeadlineExceeded, "tool catalog refresh deadline exceeded")
	case errors.Is(err, context.Canceled):
		return pluginsdk.ErrCode(pluginsdk.CodeCallCancelled, "tool catalog refresh cancelled")
	default:
		return err
	}
}
