package localwebhook

import (
	"camctl/local/localconf"
	"camctl/local/locallog"
	"camctl/local/localproxy"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type WebhookLogHandler struct {
	log       *zap.Logger
	conf      *localconf.Config
	bufferLog *locallog.BuffLog
}

func NewWebhookLogHandler(logger *zap.Logger, config *localconf.Config, buffLog *locallog.BuffLog) *WebhookLogHandler {
	res := WebhookLogHandler{log: logger, conf: config, bufferLog: buffLog}
	return &res
}

func (h *WebhookLogHandler) ServeHTTP(c *gin.Context) {
	mess := c.Request.FormValue("mess")
	if len(mess) > 0 {
		if !h.conf.IsTrustedIP(c.Request.RemoteAddr) {
			h.log.Sugar().Errorf("forbidden by remote ip %s", c.Request.RemoteAddr)
			localproxy.Error(c, "forbidden", http.StatusForbidden)
		} else {
			h.bufferLog.Log.Sugar().Info(mess)
			localproxy.Error(c, "accept", http.StatusOK)
		}
	} else {
		entries := h.bufferLog.Buffer(200)
		var sb strings.Builder
		for _, entry := range entries {
			time.Now().Format("2006-01-02 15:04:05")
			sb.WriteString(entry.Time.Format("2006-01-02 15:04:05"))
			sb.WriteString("  \t")
			sb.WriteString(entry.Message)
			sb.WriteString("\n")
		}
		localproxy.Error(c, sb.String(), http.StatusOK)
	}
}
