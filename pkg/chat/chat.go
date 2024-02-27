package chat

import (
	"context"
	"fmt"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/aidea-chat-server/pkg/repo"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/glacier/infra"
	"github.com/mylxsw/go-utils/array"
	"github.com/sashabaranov/go-openai"
	"strings"
	"time"
)

// Chatter represents a chat completion API
type Chatter struct {
	conf   *config.Config   `autowire:"@"`
	openai *OpenAIClient    `autowire:"@"`
	repo   *repo.Repository `autowire:"@"`

	// models model_id => model mapping
	models map[string]config.Model
}

// NewChatter creates a new Chatter instance
func NewChatter(resolver infra.Resolver) *Chatter {
	chatter := &Chatter{}
	resolver.MustAutoWire(chatter)

	chatter.models = array.ToMap(chatter.conf.Models, func(item config.Model, _ int) string {
		return item.ID
	})

	return chatter
}

// ChatStream for handling streaming chat requests
func (chat *Chatter) ChatStream(ctx context.Context, req Request) (<-chan StreamResponse, error) {
	// 根据 robotID 获取对应的模型
	robot, err := chat.repo.Robot.GetRobotByID(ctx, req.RobotID)
	if err != nil {
		return nil, fmt.Errorf("invalid robot: %w", err)
	}

	// TODO 根据机器人的类型进行不同的处理
	if robot.Type != repo.RobotTypeModelDriven {
		return nil, fmt.Errorf("unsupported robot type: %x", robot.Type)
	}

	// 查询模型配置信息
	model, ok := chat.models[robot.Model]
	if !ok {
		return nil, fmt.Errorf("model not found: %s", robot.Model)
	}

	// 确保上下文长度满足要求
	req.Messages, _, err = ReduceContextByTokens(req.Messages, model.ID, model.MaxContextForInput())
	if err != nil {
		return nil, ErrContextExceedLimit
	}

	promptTokenCount, _ := MessageTokenCount(req.Messages, model.ID)
	usage := Usage{
		PromptTokens: int64(promptTokenCount),
	}

	// create openai request
	openaiRequest := openai.ChatCompletionRequest{
		Model:  model.ID,
		Stream: true,
		Messages: array.Map(req.Messages, func(item Message, _ int) openai.ChatCompletionMessage {
			// If the model supports vision, the message content needs to be converted into the corresponding format
			if model.SupportVision() {
				contents := array.Map(item.MultipartContents, func(content *MultipartContent, _ int) openai.ChatMessagePart {
					part := openai.ChatMessagePart{Text: content.Text, Type: openai.ChatMessagePartType(content.Type)}
					if openai.ChatMessagePartType(content.Type) == openai.ChatMessagePartTypeImageURL {
						url := content.ImageURL.URL
						part.ImageURL = &openai.ChatMessageImageURL{
							URL:    url,
							Detail: openai.ImageURLDetail(content.ImageURL.Detail),
						}
					}

					return part
				})

				return openai.ChatCompletionMessage{
					Role:         item.Role,
					MultiContent: contents,
				}
			}

			return openai.ChatCompletionMessage{
				Role:    item.Role,
				Content: item.Content,
			}
		}),
	}

	startTime := time.Now()

	stream, err := chat.openai.ChatStream(ctx, openaiRequest)
	if err != nil {
		if strings.Contains(err.Error(), "content management policy") {
			log.With(err).Errorf("violation of Azure OpenAI content management policy")
			return nil, ErrContentFilter
		}

		return nil, err
	}

	res := make(chan StreamResponse)
	go func() {
		defer close(res)

		replyText := ""
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-stream:
				if !ok {
					return
				}

				if data.Code != "" {
					res <- StreamResponse{
						ErrorMessage: data.ErrorMessage,
						ErrorCode:    data.Code,
					}
					return
				}

				if usage.FirstLetterDelay == 0 {
					usage.FirstLetterDelay = time.Since(startTime).Milliseconds()
				}

				resp := StreamResponse{
					ID:      data.ChatResponse.ID,
					Created: data.ChatResponse.Created,
					Choices: []StreamChoice{
						{
							Index: 0,
							Delta: Delta{
								Content: array.Reduce(
									data.ChatResponse.Choices,
									func(carry string, item openai.ChatCompletionStreamChoice) string {
										return carry + item.Delta.Content
									},
									"",
								),
								Role: "assistant",
							},
							FinishReason: string(data.ChatResponse.Choices[len(data.ChatResponse.Choices)-1].FinishReason),
						},
					},
				}

				replyText += resp.DeltaText()
				tokenCount, _ := MessageTokenCount(
					append(req.Messages, Message{
						Role:    "assistant",
						Content: replyText,
					}),
					model.ID,
				)

				usage.ConsumeInMilli = time.Since(startTime).Milliseconds()
				if tokenCount > 0 {
					usage.CompletionTokens = int64(tokenCount - promptTokenCount)
					usage.TotalTokens = int64(tokenCount)
				}

				resp.Usage = &usage
				res <- resp
			}
		}
	}()

	return res, nil
}
