package chat

import (
	"encoding/json"
	"time"
)

type StreamResponse struct {
	// ID A unique identifier for the chat completion.
	ID string `json:"id"`
	// Created The Unix timestamp (in seconds) of when the chat completion was created.
	Created int64 `json:"created"`
	// Choices A list of chat completion choices. Can be more than one if n is greater than 1.
	Choices []StreamChoice `json:"choices"`

	// Usage only the last message contains Usage information
	Usage *Usage `json:"usage,omitempty"`

	// ErrorCode An error code if the model encounters an error while generating the response.
	ErrorCode string `json:"error_code,omitempty"`
	// Error An error message if the model encounters an error while generating the response.
	ErrorMessage string `json:"error_message,omitempty"`
}

type Usage struct {
	CompletionTokens int64 `json:"completion_tokens,omitempty"`
	PromptTokens     int64 `json:"prompt_tokens,omitempty"`
	TotalTokens      int64 `json:"total_tokens,omitempty"`
}

func (resp StreamResponse) JSON() string {
	res, _ := json.Marshal(resp)
	return string(res)
}

// NewStreamResponse creates a new StreamResponse with the given id, delta, and finishReason.
func NewStreamResponse(id string, delta string, finishReason string) StreamResponse {
	return StreamResponse{
		ID:      id,
		Created: time.Now().Unix(),
		Choices: []StreamChoice{
			{
				Index: 0,
				Delta: Delta{
					Role:    "assistant",
					Content: delta,
				},
				FinishReason: finishReason,
			},
		},
	}
}

func NewSystemStreamResponse(id string, delta string, finishReason string) StreamResponse {
	return StreamResponse{
		ID:      id,
		Created: time.Now().Unix(),
		Choices: []StreamChoice{
			{
				Index: 0,
				Delta: Delta{
					Role:    "system",
					Content: delta,
				},
				FinishReason: finishReason,
			},
		},
	}
}

// DeltaText returns the delta content of the first choice.
func (resp StreamResponse) DeltaText() string {
	if len(resp.Choices) > 0 {
		return resp.Choices[0].Delta.Content
	}

	return ""
}

type StreamChoice struct {
	// Index The index of the choice in the list of choices.
	Index int `json:"index"`
	// Delta A chat completion delta generated by streamed model responses.
	Delta Delta `json:"delta"`
	// FinishReason The reason the model stopped generating tokens.
	// This will be stop if the model hit a natural stop point or a provided stop sequence,
	// length if the maximum number of tokens specified in the request was reached,
	// content_filter if content was omitted due to a flag from our content filters,
	// tool_calls if the model called a tool, or function_call (deprecated) if the model called a function
	FinishReason string `json:"finish_reason,omitempty"`
}

type Delta struct {
	// Content The contents of the chunk message.
	Content string `json:"content"`
	// Role The role of the author of this message.
	Role string `json:"role,omitempty"`
	// ToolCalls The name and arguments of a function that should be called, as generated by the model.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	Index int `json:"index"`
	// ID The ID of the tool call.
	ID string `json:"id"`
	// Type The type of the tool. Currently, only function is supported
	Type string `json:"type"`
}

type Function struct {
	// Name The name of the function to call.
	Name string `json:"name"`
	// Args The arguments to call the function with, as generated by the model in JSON format.
	// Note that the model does not always generate valid JSON, and may hallucinate parameters not defined by your function schema.
	// Validate the arguments in your code before calling your function.
	Arguments string `json:"arguments"`
}
