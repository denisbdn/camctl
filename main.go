package main

import (
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"camctl/local/localconf"
	"camctl/local/localffmpeg"
	"camctl/local/locallog"
	"camctl/local/localproxy"
	"camctl/local/localserv"
	"camctl/local/localtmpl"
	"camctl/local/localwebhook"
	"camctl/local/localws"
)

const (
	// MaxCacheTimeout is meta time live - it is for meta files: m3u8, mdp, init chanks
	MaxCacheTimeout time.Duration = 24 * time.Hour

	// WaitDataInCache max time wait from cache in get > stream chank duration
	WaitDataInCache time.Duration = 3 * time.Second
)

func main() {

	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.StampNano)
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	loggerStream, _ := cfg.Build()
	blStream := locallog.NewBuffLog(loggerStream, 2000)
	defer blStream.Close()

	loggerWebhook, _ := cfg.Build()
	blWebhook := locallog.NewBuffLog(loggerWebhook, 200)
	defer blWebhook.Close()

	conf := localconf.NewConfig(blStream.Log)

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

	server := localserv.NewServer(blStream.Log)

	staticDir, err := filepath.Abs(*conf.Static)
	if err != nil {
		blStream.Log.Error("dir with static files", zap.Error(err))
		return
	}
	server.Engine.StaticFS("/static", http.Dir(staticDir))
	server.Engine.StaticFile("/", filepath.Join(staticDir, "index.html"))
	server.Engine.StaticFile("/menu.html", filepath.Join(staticDir, "menu.html"))

	var wg sync.WaitGroup

	proxy := localproxy.NewItems(&wg, blStream.Log, conf, time.Duration(*conf.ChankDur)*time.Second*2, MaxCacheTimeout, WaitDataInCache)
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

	stream := localffmpeg.NewStreamHandler(blStream.Log, conf, proxy)
	server.Engine.GET("/stream/start/:user/:cam", stream.ServeHTTP)
	server.Engine.POST("/stream/start/:user/:cam", stream.ServeHTTP)
	server.Engine.GET("/stream/stop/:user/:cam", stream.ServeHTTP)
	server.Engine.POST("/stream/stop/:user/:cam", stream.ServeHTTP)

	server.Engine.StaticFS("/history", http.Dir(*conf.StoreDir))

	storage := localffmpeg.NewStorageHandler(blStream.Log, conf)
	server.Engine.GET("/storage/start/:user/:cam", storage.ServeHTTP)
	server.Engine.GET("/storage/stop/:user/:cam", storage.ServeHTTP)

	file := localproxy.NewFiles(&wg, blStream.Log, conf, time.Duration(*conf.ChankDur)*time.Duration(*conf.Chanks)*2*time.Second)
	server.Engine.GET("/allhistory", file.ServeHTTP)
	server.Engine.POST("/allhistory", file.ServeHTTP)
	server.Engine.GET("/allhistory/:user", file.ServeHTTP)
	server.Engine.POST("/allhistory/:user", file.ServeHTTP)

	tmplHandler := localtmpl.NewTmplHandlers(server.Engine, blStream.Log, conf, proxy, stream, storage)

	wsHandler := localws.NewWebsocketLog(stream, storage, blStream.Log)
	server.Engine.GET("/ws", wsHandler.ServeHTTP)

	// ??????????
	server.Engine.GET("/time", tmplHandler.TimeHandler)
	server.Engine.HEAD("/time", tmplHandler.TimeHandler)
	server.Engine.OPTIONS("/time", tmplHandler.TimeHandler)

	// webhook log
	webhookLogHandler := localwebhook.NewWebhookLogHandler(blStream.Log, conf, blWebhook)
	server.Engine.GET("/webhooklog", webhookLogHandler.ServeHTTP)
	server.Engine.POST("/webhooklog", webhookLogHandler.ServeHTTP)

	blStream.Log.Sugar().Info("Start server")

	server.Engine.Run(*conf.Addr)

	proxy.Close()
	file.Close()

	wg.Wait()

	blStream.Log.Sugar().Info("Stop server")
}
