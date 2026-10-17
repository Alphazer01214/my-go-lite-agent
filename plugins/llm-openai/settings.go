package main

type settingsGetOut struct {
	Values map[string]any `json:"values"`
}

type settingsSaveIn struct {
	Values struct {
		BaseURL string `json:"base_url,omitempty"`
		APIKey  string `json:"api_key,omitempty"`
		Model   string `json:"model,omitempty"`
	} `json:"values"`
}

type settingsSaveOut struct {
	Saved bool `json:"saved"`
}

func (p *plugin) settingsGet() settingsGetOut {
	c := p.snapshot()
	return settingsGetOut{Values: map[string]any{
		"base_url": c.BaseURL,
		"model":    c.Model,
		"api_key":  map[string]any{"configured": c.APIKey != ""},
	}}
}

func (p *plugin) settingsSave(in settingsSaveIn) (settingsSaveOut, error) {
	_, err := p.configSet(configSetIn{
		BaseURL: in.Values.BaseURL,
		APIKey:  in.Values.APIKey,
		Model:   in.Values.Model,
	})
	return settingsSaveOut{Saved: err == nil}, err
}
