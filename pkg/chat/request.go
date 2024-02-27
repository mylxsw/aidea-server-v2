package chat

// Request represents a request structure for chat completion API.
type Request struct {
	RobotID   string   `json:"robot_id"`
	Messages  Messages `json:"messages"`
	MaxTokens int      `json:"max_tokens,omitempty"`
}
