package misc

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/go-utils/ternary"
	"net/http"
	"sync"
)

type StreamWriter struct {
	ws         *websocket.Conn
	r          *http.Request
	w          http.ResponseWriter
	enableCors bool

	once     sync.Once
	sseReady bool
	debug    bool
}

var corsHeaders = http.Header{
	"Access-Control-Allow-Origin":  []string{"*"},
	"Access-Control-Allow-Headers": []string{"*"},
	"Access-Control-Allow-Methods": []string{"GET,POST,OPTIONS,HEAD,PUT,PATCH,DELETE"},
}

type WSError struct {
	Code  int    `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
}

func (e WSError) JSON() []byte {
	data, _ := json.Marshal(e)
	return data
}

func (sw *StreamWriter) Close() {
	if sw.debug {
		log.Debugf("close stream writer")
	}

	if sw.ws != nil {
		NoError(sw.ws.Close())
	} else {
		if sw.sseReady {
			// 写入结束标志
			_, _ = sw.w.Write([]byte("data: [DONE]\n\n"))
			if f, ok := sw.w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
}

type InitRequest[T any] interface {
	Init() T
}

func NewStreamWriter[T InitRequest[T]](useWebSocket bool, enableCors bool, r *http.Request, w http.ResponseWriter) (*StreamWriter, *T, error) {
	sw := &StreamWriter{
		r:          r,
		w:          w,
		enableCors: enableCors,
	}

	var req T
	if useWebSocket {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		}

		if wsConn, err := upgrader.Upgrade(w, r, ternary.If(enableCors, corsHeaders, http.Header{})); err != nil {
			sw.writeJSON(NewStreamError(fmt.Errorf("upgrade websocket failed: %v", err)), http.StatusInternalServerError)
			return nil, nil, err
		} else {
			sw.ws = wsConn

			if sw.debug {
				log.Debugf("websocket connected: %s", wsConn.RemoteAddr())
			}

			// 读取第一条消息，用于获取用户输入
			_, msg, err := wsConn.ReadMessage()
			if err != nil {
				NoError(sw.WriteStream(NewStreamError(fmt.Errorf("read websocket message failed: %v", err))))
				NoError(wsConn.Close())
				return nil, nil, err
			}

			if err := json.Unmarshal(msg, &req); err != nil {
				NoError(sw.WriteStream(NewStreamErrorWithCode(fmt.Errorf("invalid request: %v", err), http.StatusBadRequest)))
				NoError(wsConn.Close())
				return nil, nil, err
			}
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sw.writeJSON(NewStreamErrorWithCode(fmt.Errorf("invalid request: %v", err), http.StatusBadRequest), http.StatusBadRequest)
			return nil, nil, err
		}
	}

	req = req.Init()
	return sw, &req, nil
}

func (sw *StreamWriter) initSSE() {
	if sw.ws != nil {
		return
	}

	sw.once.Do(func() {
		sw.sseReady = true

		if sw.debug {
			log.Debug("init sse")
		}

		sw.wrapRawResponse(sw.w, func() {
			sw.w.Header().Set("Content-Type", "text/event-stream")
			sw.w.Header().Set("Cache-Control", "no-cache")
			sw.w.Header().Set("Connection", "keep-alive")
		})
	})
}

func (sw *StreamWriter) WriteErrorStream(err error, statusCode int) error {
	return sw.WriteStream(NewStreamErrorWithCode(err, statusCode))
}

func (sw *StreamWriter) WriteStream(payload any) error {
	var data []byte

	if str, ok := payload.(string); ok {
		data = []byte(str)
	} else {
		data, _ = json.Marshal(payload)
	}

	if sw.debug {
		log.Debugf("write stream: %s", string(data))
	}

	if sw.ws != nil {
		return sw.ws.WriteMessage(websocket.TextMessage, data)
	}

	sw.initSSE()

	if _, err := sw.w.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
		return err
	}

	if f, ok := sw.w.(http.Flusher); ok {
		f.Flush()
	}

	return nil
}

func (sw *StreamWriter) wrapRawResponse(w http.ResponseWriter, cb func()) {
	// 允许跨域
	if sw.enableCors {
		for k, v := range corsHeaders {
			for _, v1 := range v {
				w.Header().Set(k, v1)
			}
		}
	}

	cb()
}

func (sw *StreamWriter) writeJSON(payload any, statusCode int) {
	sw.wrapRawResponse(sw.w, func() {
		data, err := json.Marshal(payload)
		if err != nil {
			sw.w.WriteHeader(http.StatusInternalServerError)
			_, _ = sw.w.Write(StreamError{Error: err.Error()}.ToJSON())
			return
		}

		sw.w.Header().Set("Content-Type", "application/json; charset=utf-8")
		sw.w.WriteHeader(statusCode)
		NoError2(sw.w.Write(data))
	})
}

type StreamError struct {
	Code  int    `json:"code"`
	Error string `json:"error"`
}

func NewStreamError(err error) StreamError {
	return StreamError{Error: err.Error(), Code: http.StatusInternalServerError}
}

func NewStreamErrorWithCode(err error, code int) StreamError {
	return StreamError{Error: err.Error(), Code: code}
}

func (resp StreamError) ToJSON() []byte {
	data, _ := json.Marshal(resp)
	return data
}
