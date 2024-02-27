package chat

import (
	"context"
	"strconv"
	"time"
)

// Chatter represents a chat completion API
type Chatter struct {
}

// New creates a new Chatter instance
func New() *Chatter {
	return &Chatter{}
}

// ChatStream for handling streaming chat requests
func (chat *Chatter) ChatStream(ctx context.Context, req Request) (<-chan StreamResponse, error) {
	// TODO 根据 robotID 获取对应的模型
	model := req.RobotID

	promptTokenCount, _ := MessageTokenCount(req.Messages, model)
	stream := make(chan StreamResponse)
	// TODO
	go func() {
		defer close(stream)

		fakeMessage := []rune("白炅翰和朴信爱这对韩国夫妇是信阳师范大学的韩语老师，夫妇二人从在北京求学相识起，就与中国结下不解之缘。如今的他们不仅传授知识，更积极推动中韩文化交流。走近他们，感受一场关于爱、文化和传承的心灵之旅。")

		replyText := ""
		for i := 0; i < len(fakeMessage); i++ {
			time.Sleep(30 * time.Millisecond)

			replyText += string(fakeMessage[i])
			state := append(req.Messages, Message{
				Role:    "assistant",
				Content: replyText,
			})

			resp := StreamResponse{
				ID:      strconv.Itoa(i),
				Created: time.Now().Unix(),
				Choices: []StreamChoice{
					{
						Index: 0,
						Delta: Delta{
							Content: string(fakeMessage[i]),
							Role:    "assistant",
						},
					},
				},
			}

			tokenCount, _ := MessageTokenCount(state, model)
			if tokenCount > 0 && promptTokenCount > 0 {
				resp.Usage = &Usage{
					PromptTokens:     int64(promptTokenCount),
					CompletionTokens: int64(tokenCount - promptTokenCount),
					TotalTokens:      int64(tokenCount),
				}
			}

			select {
			case stream <- resp:
			case <-ctx.Done():
				return
			}
		}
	}()

	return stream, nil
}
