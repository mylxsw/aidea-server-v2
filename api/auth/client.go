package auth

type ClientInfo struct {
	Version         string `json:"version"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platform_version"`
	Language        string `json:"language"`
	IP              string `json:"ip"`
}

// IsIOS 返回客户端是否是 IOS 平台
func (inf ClientInfo) IsIOS() bool {
	return inf.Platform == "ios"
}
