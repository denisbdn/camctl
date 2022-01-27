package localws

import (
	"camctl/local/localffmpeg"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// WebsocketLog struct describe websocket handler
type WebsocketLog struct {
	ffmpegs    *localffmpeg.StreamHandler
	log        *zap.Logger
	wsUpgrader websocket.Upgrader
}

// NewWebsocketLog build WebsocketLog object
func NewWebsocketLog(ffmpegs *localffmpeg.StreamHandler, log *zap.Logger) *WebsocketLog {
	res := WebsocketLog{ffmpegs, log, websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}}
	return &res
}

// responce code
const (
	OK       = 0
	BadParam = 1
	NotFound = 2
)

// JSONResponce struct describe websocket responce
type JSONResponce struct {
	Errno int    `json:"errno,omitempty"`
	Error string `json:"error,omitempty"`
}

// JSONRequest struct describe websocket request
type JSONRequest struct {
	Method string        `json:"method,omitempty"`
	Entry  zapcore.Entry `json:"entry,omitempty"`
}

func (h *WebsocketLog) ServeHTTP(c *gin.Context) {
	ws, errUpgrade := h.wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if errUpgrade != nil {
		h.log.Sugar().Error("websocket Upgrade", zap.Error(errUpgrade))
		return
	}
	defer ws.Close()
	var init struct {
		Method string `json:"method"`
		Path   string `json:"path"`
	}
	errRead := ws.ReadJSON(&init)
	if errRead != nil {
		h.log.Sugar().Error("websocket read", zap.Error(errRead))
		ws.WriteJSON(JSONResponce{Errno: BadParam, Error: "Bad read: {\"method\": \"Init\", \"path\": \"SomeKey\"}"})
		return
	}
	if init.Method != "Init" {
		h.log.Sugar().Error("websocket method", zap.Error(errRead))
		ws.WriteJSON(JSONResponce{Errno: BadParam, Error: "Bad method: {\"method\": \"Init\", \"path\": \"SomeKey\"}"})
		return
	}
	ffmpeg := h.ffmpegs.GetProcArgs(init.Path)
	if ffmpeg == nil {
		h.log.Sugar().Error("websocket ffmpeg not found", zap.Error(errRead))
		ws.WriteJSON(JSONResponce{Errno: NotFound, Error: "Stream not found"})
		return
	}
	cn := make(chan zapcore.Entry, 100)
	entries := ffmpeg.Log.AddSubscriberBuffer(cn, 10)
	defer ffmpeg.Log.DelSubscriber(cn)
	for _, entry := range entries {
		ws.WriteJSON(JSONRequest{Method: "Log", Entry: entry})
	}
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()
	isContinue := true
	for isContinue {
		select {
		case entry, ok := <-cn:
			if ok {
				ws.WriteJSON(JSONRequest{Method: "Log", Entry: entry})
			} else {
				if ffmpeg.Log.DelSubscriber(cn) != 0 {
					close(cn)
				}
				ws.WriteJSON(JSONRequest{Method: "Log", Entry: zapcore.Entry{Level: zap.InfoLevel, Time: time.Now(), Message: "Close Logger"}})
				isContinue = false
			}
		case <-ticker.C:
			if errPing := ws.WriteJSON(JSONRequest{Method: "Ping", Entry: zapcore.Entry{Level: zap.InfoLevel, Time: time.Now(), Message: "Service message"}}); errPing != nil {
				h.log.Sugar().Error("websocket ping", zap.Error(errPing))
				if ffmpeg.Log.DelSubscriber(cn) != 0 {
					close(cn)
				}
				isContinue = false
			}
		}
	}
}
