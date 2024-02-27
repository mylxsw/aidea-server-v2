package chat

import (
	"context"
	"errors"
	"fmt"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/aidea-chat-server/pkg/proxy"
	"github.com/sashabaranov/go-openai"
	"io"
	"time"
)

type OpenAIClient struct {
	conf  config.OpenAIConfig
	proxy *proxy.Proxy
}

func NewOpenAIClient(conf config.OpenAIConfig, pp *proxy.Proxy) *OpenAIClient {
	return &OpenAIClient{conf: conf, proxy: pp}
}

// createClient create a new openai client
func (client *OpenAIClient) createClient() *openai.Client {
	conf := openai.DefaultConfig(client.conf.APIKey)
	conf.BaseURL = client.conf.ServerURL
	conf.OrgID = client.conf.Organization

	conf.HTTPClient.Timeout = 180 * time.Second
	conf.HTTPClient.Transport = client.proxy.BuildTransport()

	if client.conf.UseAzure {
		conf.APIType = openai.APITypeAzure
		conf.APIVersion = client.conf.AzureAPIVersion
		if client.conf.AzureModelMapping != nil {
			conf.AzureModelMapperFunc = func(model string) string {
				if v, ok := client.conf.AzureModelMapping[model]; ok {
					return v
				}

				return model
			}
		}
	}

	return openai.NewClientWithConfig(conf)
}

type OpenAIStreamResponse struct {
	Code         string `json:"code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	ChatResponse *openai.ChatCompletionStreamResponse
}

// ChatStream call the OpenAI interface to initiate a streaming chat request
func (client *OpenAIClient) ChatStream(ctx context.Context, request openai.ChatCompletionRequest) (<-chan OpenAIStreamResponse, error) {
	request.Stream = true

	stream, err := client.createClient().CreateChatCompletionStream(ctx, request)
	if err != nil {
		return nil, err
	}

	res := make(chan OpenAIStreamResponse)
	go func() {
		defer func() {
			close(res)
			stream.Close()
		}()

		for {
			response, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}

			if err != nil {
				select {
				case <-ctx.Done():
				case res <- OpenAIStreamResponse{Code: "READ_STREAM_FAILED", ErrorMessage: fmt.Errorf("read stream failed: %v", err).Error()}:
				}
				return
			}

			select {
			case <-ctx.Done():
				return
			case res <- OpenAIStreamResponse{ChatResponse: &response}:
			}
		}
	}()

	return res, nil
}
