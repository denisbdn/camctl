package main

import (
	"net/http"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"camctl/local/localconf"
	"camctl/local/localffmpeg"
	"camctl/local/locallog"
	"camctl/local/localproxy"
	"camctl/local/localserv"
	"camctl/local/localtmpl"
	"camctl/local/localws"
)

const (
	// CacheTimeout is chank time live
	CacheTimeout time.Duration = 60 * time.Second

	// MaxCacheTimeout is meta time live
	MaxCacheTimeout time.Duration = 24 * time.Hour

	// WaitDataInCache max time wait from cache in get
	WaitDataInCache time.Duration = 3 * time.Second
)

func main() {

	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.StampNano)
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	logger, _ := cfg.Build()

	bl := locallog.NewBuffLog(logger, 2000)
	defer bl.Close()

	conf := localconf.NewConfig(bl.Log)

	// go func() {
	// 	cn := make(chan zapcore.Entry, 0)
	// 	res := bl.AddSubscriber(cn)
	// 	fmt.Printf("AddSubscriber: %d\n", res)
	// 	i := 0
	// 	for m := range cn {
	// 		fmt.Printf("recive: %s\n", m.Message)
	// 		i++
	// 		if i == 2 {
	// 			break
	// 		}
	// 	}
	// 	res = bl.DelSubscriber(cn)
	// 	fmt.Printf("DelSubscriber: %d\n", res)
	// 	close(cn)
	// }()
	// time.Sleep(2 * time.Second)
	// bl.Log.Info("1")
	// time.Sleep(2 * time.Second)
	// bl.Log.Info("2")
	// time.Sleep(2 * time.Second)
	// bl.Log.Info("3")
	// time.Sleep(2 * time.Second)

	// arr := bl.Buffer(5)
	// fmt.Print(arr)

	server := localserv.NewServer(bl.Log)

	staticDir, err := filepath.Abs(*conf.Static)
	if err != nil {
		bl.Log.Error("dir with static files", zap.Error(err))
		return
	}
	server.Engine.StaticFS("/static", http.Dir(staticDir))
	server.Engine.StaticFile("/", filepath.Join(staticDir, "index.html"))
	server.Engine.StaticFile("/menu.html", filepath.Join(staticDir, "menu.html"))

	proxy := localproxy.NewItems(bl.Log, conf, CacheTimeout, MaxCacheTimeout, WaitDataInCache)
	server.Engine.GET("/info/:user/:cam", proxy.ServeHTTP)
	server.Engine.POST("/info/:user/:cam", proxy.ServeHTTP)
	server.Engine.GET("/info/:user", proxy.ServeHTTP)
	server.Engine.POST("/info/:user", proxy.ServeHTTP)
	server.Engine.GET("/info", proxy.ServeHTTP)
	server.Engine.POST("/info", proxy.ServeHTTP)
	server.Engine.GET("/get/:user/:cam/:file", proxy.ServeHTTP)
	server.Engine.POST("/get/:user/:cam/:file", proxy.ServeHTTP)
	server.Engine.PUT("/put/:user/:cam/:file", proxy.ServeHTTP)
	server.Engine.POST("/put/:user/:cam/:file", proxy.ServeHTTP)
	server.Engine.DELETE("/put/:user/:cam/:file", proxy.ServeHTTP)
	server.Engine.DELETE("/put/:user/:cam", proxy.ServeHTTP)
	server.Engine.DELETE("/put/:user", proxy.ServeHTTP)

	ffmpeg := localffmpeg.NewFfmpegHandler(bl.Log, conf, proxy)
	server.Engine.GET("/ffmpeg/start/:user/:cam", ffmpeg.ServeHTTP)
	server.Engine.POST("/ffmpeg/start/:user/:cam", proxy.ServeHTTP)
	server.Engine.GET("/ffmpeg/stop/:user/:cam", ffmpeg.ServeHTTP)
	server.Engine.POST("/ffmpeg/stop/:user/:cam", proxy.ServeHTTP)

	tmplHandler := localtmpl.NewTmplHandlers(server.Engine, bl.Log, conf, proxy, ffmpeg)

	wsHandler := localws.NewWebsocketLog(ffmpeg, bl.Log)
	server.Engine.GET("/ws", wsHandler.ServeHTTP)

	// время
	server.Engine.GET("/time", tmplHandler.TimeHandler)
	server.Engine.HEAD("/time", tmplHandler.TimeHandler)
	server.Engine.OPTIONS("/time", tmplHandler.TimeHandler)

	server.Engine.Run(*conf.Addr)
}
