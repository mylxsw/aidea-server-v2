package config

import "github.com/mylxsw/go-utils/array"

type Model struct {
	// ID model id
	ID string `json:"id,omitempty" yaml:"id,omitempty"`
	// Name model name
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// AvatarURL model avatar
	AvatarURL string `json:"avatar_url,omitempty" yaml:"avatar_url,omitempty"`
	// Price model price, calculated based on 1K Token
	Price int64 `json:"price,omitempty" yaml:"price,omitempty"`
	// MaxContext model max context
	MaxContext int `json:"max_context,omitempty" yaml:"max_context,omitempty"`
	// Capabilities model capabilities
	Capabilities []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
}

// ModelCapabilityVision model capability: vision
const ModelCapabilityVision = "vision"

// SupportVision whether the model supports vision
func (m Model) SupportVision() bool {
	return array.In(ModelCapabilityVision, m.Capabilities)
}

// MaxContextForInput calculate the max context for input
func (m Model) MaxContextForInput() int {
	if m.MaxContext == 0 {
		m.MaxContext = 2000
	}

	return int(float64(m.MaxContext) * 0.8)
}
