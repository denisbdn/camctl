package localserv

import (
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Server struct describe gin.Engine
type Server struct {
	Engine *gin.Engine
}

func loggerWithConfig(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timer
		start := time.Now()
		method := c.Request.Method
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Process request
		c.Next()

		latency := time.Now().Sub(start)
		logger.Sugar().Infow("requesr", "method", method, "path", path, "query", raw, "latency", latency)
	}
}

// NewServer build gin server with logger
func NewServer(logger *zap.Logger) *Server {
	res := new(Server)
	res.Engine = gin.New()
	pprof.Register(res.Engine, "dev/pprof")
	res.Engine.Use(loggerWithConfig(logger))
	return res
}
