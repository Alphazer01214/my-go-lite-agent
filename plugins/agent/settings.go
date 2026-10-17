package main

type settingsGetOut struct {
	Values map[string]any `json:"values"`
}

type settingsSaveIn struct {
	Values struct {
		DefaultScheme string `json:"default_scheme,omitempty"`
		MaxSteps      int    `json:"max_steps,omitempty"`
	} `json:"values"`
}

type settingsSaveOut struct {
	Saved bool `json:"saved"`
}

func (p *plugin) settingsGet() settingsGetOut {
	c := p.snapshot()
	return settingsGetOut{Values: map[string]any{
		"default_scheme": c.DefaultScheme,
		"max_steps":      c.MaxSteps,
	}}
}

func (p *plugin) settingsSave(in settingsSaveIn) (settingsSaveOut, error) {
	_, err := p.configSet(configSetIn{
		DefaultScheme: in.Values.DefaultScheme,
		MaxSteps:      in.Values.MaxSteps,
	})
	return settingsSaveOut{Saved: err == nil}, err
}
