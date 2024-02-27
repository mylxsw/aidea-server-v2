package auth

type ClientInfo struct {
	Version         string `json:"version,omitempty"`
	Platform        string `json:"platform,omitempty"`
	PlatformVersion string `json:"platform_version,omitempty"`
	Language        string `json:"language,omitempty"`
	IP              string `json:"ip,omitempty"`
}

// IsIOS 返回客户端是否是 IOS 平台
func (inf ClientInfo) IsIOS() bool {
	return inf.Platform == "ios"
}
