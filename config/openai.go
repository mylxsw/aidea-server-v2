package config

type OpenAIConfig struct {
	ServerURL       string `json:"server_url,omitempty" yaml:"server_url,omitempty"`
	APIKey          string `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	Organization    string `json:"organization,omitempty" yaml:"organization,omitempty"`
	UseAzure        bool   `json:"use_azure,omitempty" yaml:"use_azure,omitempty"`
	AzureAPIVersion string `json:"azure_api_version,omitempty" yaml:"azure_api_version,omitempty"`

	// AzureModelMapping azure model mapping
	// Key: OpenAI model name, Value: Azure model name
	AzureModelMapping map[string]string `json:"azure_model_mapping,omitempty" yaml:"azure_model_mapping,omitempty"`
}
