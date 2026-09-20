package plugin

type Manifest struct {
	Name string `json:"name"`
	Version string `json:"version"`
	Protocol int `json:"protocol"`
	Autostart bool `json:"autostart,omitempty"`
	// Entry: for example agent or agent.exe
	Entry string `json:"entry"`
	TimeoutMs int `json:"timeout_ms,omitempty"`

	// Provides: the capability this plugin provides
	Provides []string `json:"provides,omitempty"`
	// Requires: the capability this plugin requires
	Requires []string `json:"requires,omitempty"`
	// DependsOn: the plugin names this plugin depends on
	DependsOn []string `json:"depends_on,omitempty"`
	// HostFaces: the host faces this plugin requires, for example commands/config
	HostFaces []string `json:"host_faces,omitempty"`
}

type UI struct {
	// Entry: for example main.js
	Entry string `json:"entry"`
}

type Command struct {

}

func 