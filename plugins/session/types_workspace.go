package main

type workspaceListOut struct {
	Workspaces []workspaceRecord `json:"workspaces"`
	SelectedID string            `json:"selected_id,omitempty"`
}

type workspaceCreateIn struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
}

type workspaceCreateOut struct {
	Workspace workspaceRecord `json:"workspace"`
	Created   bool            `json:"created"`
}

type workspaceSelectIn struct {
	WorkspaceID string `json:"workspace_id"`
}

type workspaceSelectOut struct {
	WorkspaceID string `json:"workspace_id"`
}

type workspaceRenameIn struct {
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
}

type workspaceRenameOut struct {
	Workspace workspaceRecord `json:"workspace"`
}

type workspaceRemoveIn struct {
	WorkspaceID string `json:"workspace_id"`
}

type workspaceRemoveOut struct {
	WorkspaceID string `json:"workspace_id"`
	Removed     bool   `json:"removed"`
	SelectedID  string `json:"selected_id,omitempty"`
}

type workspaceCurrentOut struct {
	Workspace *workspaceRecord `json:"workspace,omitempty"`
}
