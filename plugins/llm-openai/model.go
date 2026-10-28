package main

type ModelConfig struct {
	Name    string `json:"name"`
	BaseUrl string `json:"base_url"`
	ApiKey  string `json:"api_key"`
}

type ModelCapabilities struct {
	Streaming  bool `json:"streaming"`
	Tools      bool `json:"tools"`
	Vision     bool `json:"vision"`
	Audio      bool `json:"audio"`
	Reasoning  bool `json:"reasoning"`
	JsonSchema bool `json:"json_schema"`
}
