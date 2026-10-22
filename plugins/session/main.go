package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func toSDKError(err error) error {
	if err == nil {
		return nil
	}
	if c, ok := err.(coded); ok {
		return pluginsdk.ErrCode(c.code, c.msg)
	}
	return err
}

func handle(fn func(ctx context.Context, req *pluginsdk.Request) (any, error)) pluginsdk.HandleFunc {
	return func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		out, err := fn(ctx, req)
		if err != nil {
			return nil, toSDKError(err)
		}
		if out == nil {
			return json.RawMessage("null"), nil
		}
		b, err := json.Marshal(out)
		if err != nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
		}
		return b, nil
	}
}

func decode[T any](req *pluginsdk.Request, dst *T) error {
	if len(req.Payload) == 0 {
		return errBadArguments("payload is required")
	}
	if err := json.Unmarshal(req.Payload, dst); err != nil {
		return errBadArguments(err.Error())
	}
	return nil
}

func main() {
	p := newPlugin(dataDir())
	p.loadAll()
	// One-time write pass: bind sessions logged before the workspace registry
	// existed. Read handlers must not mutate the log or the registry.
	p.backfillWorkspaces()

	s := pluginsdk.NewPlugin("session")
	p.sdk = s
	p.setUIPublisher(s.WebUI())
	p.startStreamOutbox()

	s.Register("session", "create", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in createIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.create(ctx, in)
	})).WithInfo("create", "create a new session"))

	s.Register("session", "append", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in appendIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.append(ctx, in)
	})).WithInfo("append", "append facts to a session"))

	s.Register("session", "query", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in queryIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.query(in)
	})).WithInfo("query", "query session facts"))

	s.Register("session", "derive", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in deriveIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.derive(in)
	})).WithInfo("derive", "project Model Context messages from the log"))

	s.Register("session", "list", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in listIn
		if len(req.Payload) > 0 {
			if err := decode(req, &in); err != nil {
				return nil, err
			}
		}
		return p.list(in.WorkspaceID, in.IncludeSubagents)
	})).WithInfo("list", "list sessions"))

	s.Register("session", "info", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in infoIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.info(in)
	})).WithInfo("info", "session metadata and counts"))

	s.Register("session", "state", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in stateIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.state(ctx, in)
	})).WithInfo("state", "read rebuildable UI state"))

	s.Register("session", "messages", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in messagesIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.messages(in)
	})).WithInfo("messages", "read one older page of Chat messages"))

	s.Register("session", "facts", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in factsIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.facts(in)
	})).WithInfo("facts", "read one older page of audit Facts"))

	s.Register("session", "dock", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in stateIn
		if len(req.Payload) > 0 {
			if err := decode(req, &in); err != nil {
				return nil, err
			}
		}
		return p.dockState(ctx, in)
	})).WithInfo("dock", "read the bottom dock projection for the effective session"))

	s.Register("session", "select", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in selectIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.selectSession(ctx, in)
	})).WithInfo("select", "select the session shown by the UI mounts"))

	s.Register("session", "close", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in closeIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.closeSession(ctx, in)
	})).WithInfo("close", "close one open session pane"))

	s.Register("session", "refresh", pluginsdk.NewHandler(handle(func(ctx context.Context, _ *pluginsdk.Request) (any, error) {
		// Republishes whatever is currently selected; it takes no input and
		// changes no selection of its own.
		p.publishForSelection(ctx)
		return map[string]bool{"published": true}, nil
	})).WithInfo("refresh", "republish UI state after workspace selection"))

	s.Register("session", "workspace.list", pluginsdk.NewHandler(handle(func(context.Context, *pluginsdk.Request) (any, error) {
		records, selected := p.workspaces.list()
		return workspaceListOut{Workspaces: records, SelectedID: selected}, nil
	})).WithInfo("workspace.list", "list workspaces owned by the session plugin"))

	s.Register("session", "workspace.create", pluginsdk.NewHandler(handle(func(_ context.Context, req *pluginsdk.Request) (any, error) {
		var in workspaceCreateIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		record, created, err := p.workspaces.ensure(in.Path, in.Name, true)
		if err != nil {
			return nil, errCoded(codeWorkspaceInvalid, err.Error())
		}
		return workspaceCreateOut{Workspace: record, Created: created}, nil
	})).WithInfo("workspace.create", "register and select an existing directory"))

	s.Register("session", "workspace.select", pluginsdk.NewHandler(handle(func(_ context.Context, req *pluginsdk.Request) (any, error) {
		var in workspaceSelectIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		if err := p.selectWorkspace(in.WorkspaceID); err != nil {
			return nil, errCoded(codeWorkspaceNotFound, err.Error())
		}
		return workspaceSelectOut{WorkspaceID: in.WorkspaceID}, nil
	})).WithInfo("workspace.select", "select one workspace"))

	s.Register("session", "workspace.current", pluginsdk.NewHandler(handle(func(context.Context, *pluginsdk.Request) (any, error) {
		record, _ := p.selectedWorkspace()
		return workspaceCurrentOut{Workspace: record}, nil
	})).WithInfo("workspace.current", "read the selected workspace"))

	s.Register("session", "workspace.rename", pluginsdk.NewHandler(handle(func(_ context.Context, req *pluginsdk.Request) (any, error) {
		var in workspaceRenameIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		record, err := p.workspaces.rename(in.WorkspaceID, in.Name)
		if err != nil {
			return nil, errBadArguments(err.Error())
		}
		return workspaceRenameOut{Workspace: record}, nil
	})).WithInfo("workspace.rename", "rename one workspace"))

	s.Register("session", "workspace.remove", pluginsdk.NewHandler(handle(func(_ context.Context, req *pluginsdk.Request) (any, error) {
		var in workspaceRemoveIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		removed, selected, err := p.workspaces.remove(in.WorkspaceID)
		if err != nil {
			return nil, errHandler(err.Error())
		}
		return workspaceRemoveOut{WorkspaceID: in.WorkspaceID, Removed: removed, SelectedID: selected}, nil
	})).WithInfo("workspace.remove", "remove one workspace registration"))

	s.Register("session", "update", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in updateIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.updateSession(ctx, in)
	})).WithInfo("update", "update session name"))

	s.Register("session", "stream.update", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in streamUpdateIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.updateStream(ctx, in)
	})).WithInfo("stream.update", "update transient assistant stream state"))

	if err := s.Serve(); err != nil {
		os.Exit(1)
	}
}
