package chat

import "errors"

var (
	ErrContextExceedLimit = errors.New("context length exceeds maximum limit")
	ErrContentFilter      = errors.New("the request or response content contains sensitive words")
)
