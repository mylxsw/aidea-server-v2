package chat

import (
	"fmt"
	"strings"
)

type Message struct {
	Role              string              `json:"role"`
	Content           string              `json:"content"`
	MultipartContents []*MultipartContent `json:"multipart_content,omitempty"`
}

type MultipartContent struct {
	// Type 对于 OpenAI 来说， type 可选值为 image_url/text
	Type     string    `json:"type"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
	Text     string    `json:"text,omitempty"`
}

type ImageURL struct {
	// URL Either a URL of the image or the base64 encoded image data.
	URL string `json:"url,omitempty"`
	// Detail Specifies the detail level of the image
	// Three options, low, high, or auto, you have control over how the model processes the image and generates its textual understanding.
	// By default, the model will use the auto setting which will look at the image input size and decide if it should use the low or high setting
	//
	// - `low` will disable the “high res” model. The model will receive a low-res 512px x 512px version of the image,
	//   and represent the image with a budget of 65 tokens. This allows the API to return faster responses and consume
	//   fewer input tokens for use cases that do not require high detail.
	//
	// - `high` will enable “high res” mode, which first allows the model to see the low res image and
	//   then creates detailed crops of input images as 512px squares based on the input image size.
	//   Each of the detailed crops uses twice the jwt budget (65 tokens) for a total of 129 tokens.
	Detail string `json:"detail,omitempty"`
}

type Messages []Message

func (ms Messages) HasImage() bool {
	for _, msg := range ms {
		for _, part := range msg.MultipartContents {
			if part.ImageURL != nil && part.ImageURL.URL != "" {
				return true
			}
		}
	}

	return false
}

func (ms Messages) ToString() string {
	var msgs []string
	for _, msg := range ms {
		msgs = append(msgs, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
	}

	return strings.Join(msgs, "\n\n")
}
