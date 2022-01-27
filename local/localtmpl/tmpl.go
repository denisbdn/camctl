package localtmpl

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"camctl/local/localconf"
	"camctl/local/localffmpeg"
	"camctl/local/localproxy"
)

// TmplHandlers struct describe templates and handlers bind with its
type TmplHandlers struct {
	engine  *gin.Engine
	log     *zap.Logger
	conf    *localconf.Config
	items   *localproxy.Items
	stream  *localffmpeg.StreamHandler
	storage *localffmpeg.StorageHandler
}

func (h *TmplHandlers) loadFiles() error {
	files, err1 := ioutil.ReadDir(*h.conf.Tmpl)
	if err1 != nil {
		return err1
	}

	array := make([]string, 0)
	for _, f := range files {

		file, fileError := filepath.Abs(filepath.Join(*h.conf.Tmpl, f.Name()))
		if fileError != nil {
			h.log.Error("build streams.html", zap.Error(fileError))
		} else {
			array = append(array, file)
			switch f.Name() {
			case "dash.html":
				h.engine.GET("/dash.html", h.DashHandler)
				h.engine.POST("/dash.html", h.DashHandler)
			case "shaka.html":
				h.engine.GET("/shaka.html", h.ShakaHandler)
				h.engine.POST("/shaka.html", h.ShakaHandler)
			case "hls.html":
				h.engine.GET("/hls.html", h.HlsHandler)
				h.engine.POST("/hls.html", h.HlsHandler)
			case "raw.html":
				h.engine.GET("/raw.html", h.RawHandler)
				h.engine.POST("/raw.html", h.RawHandler)
			case "create.html":
				h.engine.GET("/create.html", h.CreateHandler)
				h.engine.POST("/create.html", h.CreateHandler)
			case "info.html":
				h.engine.GET("/info.html", h.InfoHandler)
				h.engine.POST("/info.html", h.InfoHandler)
			case "log.html":
				h.engine.GET("/streamlog.html", h.LogHandler)
				h.engine.POST("/streamlog.html", h.LogHandler)
				h.engine.GET("/storagelog.html", h.LogHandler)
				h.engine.POST("/storagelog.html", h.LogHandler)
			}
		}
	}
	h.engine.GET("/close.html", h.CloseHandler)
	h.engine.POST("/close.html", h.CloseHandler)
	h.engine.LoadHTMLFiles(array...)

	return nil
}

func formatAsDate(t time.Time) string {
	year, month, day := t.Date()
	return fmt.Sprintf("%d%02d/%02d", year, month, day)
}

// RawHandler тестовый хандлер
func (h *TmplHandlers) RawHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "raw.html", map[string]interface{}{
		"now": time.Date(2017, 07, 01, 0, 0, 0, 0, time.UTC),
	})
}

// NewTmplHandlers парсит шаблоны привязывыет урлы и строит объект TmplHandlers
func NewTmplHandlers(engine *gin.Engine, logger *zap.Logger, config *localconf.Config, items *localproxy.Items, stream *localffmpeg.StreamHandler, storage *localffmpeg.StorageHandler) *TmplHandlers {
	res := TmplHandlers{engine: engine, log: logger, conf: config, items: items, stream: stream, storage: storage}
	res.engine.Delims("{{", "}}")
	res.engine.SetFuncMap(template.FuncMap{
		"formatAsDate": formatAsDate,
	})

	res.loadFiles()

	return &res
}

// TimeHandler обработчик времени
func (h *TmplHandlers) TimeHandler(c *gin.Context) {

	c.Header("Cache-Control", "max-age=0, no-cache, no-store")
	c.Header("Pragma", "no-cache")
	c.Header("Timing-Allow-Origin", "*")
	c.Header("Access-Control-Expose-Headers", "Server,Content-Length,Date")
	c.Header("Access-Control-Allow-Headers", "origin,accept-encoding,referer")
	c.Header("Access-Control-Allow-Methods", "GET,HEAD,OPTIONS")
	c.Header("Access-Control-Allow-Origin", "*")
	// c.Header("Content-Type", "text/plain; charset=ISO-8859-1")

	mess := fmt.Sprintf("%d", time.Now().Unix())
	if strings.EqualFold(c.Request.URL.RawQuery, "iso") {
		mess = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	}
	c.Data(http.StatusOK, "text/plain; charset=ISO-8859-1", []byte(mess))
}

// Mess структура для ответа mess.html
type Mess struct {
	H1   string
	Mess string
}

// notifyDesc структура для парсинга параметров при создании вещания - локальная
type notifyDesc struct {
	URL   string `json:"url,omitempty"`
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

// streamDesc структура для парсинга параметров при создании вещания - локальная
type streamDesc struct {
	URL     string       `json:"url,omitempty"`
	User    string       `json:"user,omitempty"`
	Cam     string       `json:"cam,omitempty"`
	WorkDir string       `json:"workdir,omitempty"`
	Notify  []notifyDesc `json:"notify,omitempty"`
}

func (s *streamDesc) buildFFMPEGStartURL(host string) (string, error) {
	var sb strings.Builder
	if !strings.HasPrefix(host, "http://") {
		sb.WriteString("http://")
	}
	sb.WriteString(host)
	if strings.HasSuffix(host, "/") {
		sb.WriteString("stream/start/")
	} else {
		sb.WriteString("/stream/start/")
	}
	if len(s.User) == 0 {
		return sb.String(), fmt.Errorf("'User' is empty")
	}
	sb.WriteString(s.User)
	sb.WriteString("/")
	if len(s.Cam) == 0 {
		return sb.String(), fmt.Errorf("'Cam' is empty")
	}
	sb.WriteString(s.Cam)
	sb.WriteString("?url=")
	if len(s.URL) == 0 {
		return sb.String(), fmt.Errorf("'URL' is empty")
	}
	if _, errParse := url.ParseQuery(s.URL); errParse != nil {
		return sb.String(), fmt.Errorf("'URL' parse error %s", errParse)
	}
	sb.WriteString(url.QueryEscape(s.URL))
	for _, notify := range s.Notify {
		var add strings.Builder
		if len(notify.URL) == 0 {
			continue
		}
		if strings.Contains(notify.URL, "|") {
			return sb.String(), fmt.Errorf("'Notify.URL' contains delimeter '|'")
		}
		if _, errParse := url.ParseQuery(notify.URL); errParse != nil {
			return sb.String(), fmt.Errorf("'Notify.URL' parse error %s", errParse)
		}
		if len(notify.Key) > 0 {
			if strings.Contains(notify.Key, "|") {
				return sb.String(), fmt.Errorf("'Notify.Key' contains delimeter '|'")
			}
			add.WriteString(notify.Key)
			if len(notify.Value) > 0 {
				if strings.Contains(notify.Value, "|") {
					return sb.String(), fmt.Errorf("'Notify.Value' contains delimeter '|'")
				}
				add.WriteString("|")
				add.WriteString(notify.Value)
			}
			add.WriteString("|")
		}
		add.WriteString(notify.URL)
		sb.WriteString("&notify=")
		sb.WriteString(url.QueryEscape(add.String()))
	}
	return sb.String(), nil
}

func (s *streamDesc) buildFFMPEGStopURL(host string) (string, error) {
	var sb strings.Builder
	if !strings.HasPrefix(host, "http://") {
		sb.WriteString("http://")
	}
	sb.WriteString(host)
	if strings.HasSuffix(host, "/") {
		sb.WriteString("stream/start/")
	} else {
		sb.WriteString("/stream/start/")
	}
	if len(s.User) == 0 {
		return sb.String(), fmt.Errorf("'User' is empty")
	}
	sb.WriteString(s.User)
	sb.WriteString("/")
	if len(s.Cam) == 0 {
		return sb.String(), fmt.Errorf("'Cam' is empty")
	}
	sb.WriteString(s.Cam)
	return sb.String(), nil
}

// CreateHandler создает поток
func (h *TmplHandlers) CreateHandler(c *gin.Context) {
	data := c.Request.FormValue("data")
	if len(data) > 0 {
		h.log.Sugar().Info(data)
		var stream streamDesc
		if errUnmarshal := json.Unmarshal([]byte(data), &stream); errUnmarshal != nil {
			h.log.Error("stream", zap.Any("data", stream))
			c.HTML(http.StatusOK, "mess.html", Mess{Mess: "Error"})
		} else {
			h.log.Info("stream", zap.Any("data", stream))
			url, errBuild := stream.buildFFMPEGStartURL(fmt.Sprintf("http://127.0.0.1:%d", *h.conf.Port))
			if errBuild != nil {
				c.HTML(http.StatusOK, "mess.html", Mess{Mess: errBuild.Error()})
			} else {
				resp, errGet := http.Get(url)
				if errGet != nil {
					c.HTML(http.StatusOK, "mess.html", Mess{Mess: errGet.Error()})
					return
				}
				body, errRead := ioutil.ReadAll(resp.Body)
				if errRead == nil {
					resp.Body.Close()
					c.HTML(http.StatusOK, "mess.html", Mess{Mess: string(body)})
				} else {
					c.HTML(http.StatusOK, "mess.html", Mess{Mess: errRead.Error()})
				}
			}
		}
	} else {
		c.HTML(http.StatusOK, "create.html", nil)
	}
}

// CloseHandler закрывает поток
func (h *TmplHandlers) CloseHandler(c *gin.Context) {
	path := c.Request.FormValue("path")
	if len(path) == 0 {
		c.HTML(http.StatusOK, "mess.html", Mess{Mess: "'path' is empty"})
	} else {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		resp, errGet := http.Get(fmt.Sprintf("http://127.0.0.1:%d/stream/stop%s", *h.conf.Port, path))
		if errGet != nil {
			c.HTML(http.StatusOK, "mess.html", Mess{Mess: errGet.Error()})
			return
		}
		body, errRead := ioutil.ReadAll(resp.Body)
		if errRead == nil {
			resp.Body.Close()
			c.HTML(http.StatusOK, "mess.html", Mess{Mess: string(body)})
		} else {
			c.HTML(http.StatusOK, "mess.html", Mess{Mess: errRead.Error()})
		}
	}
}

// InfoHandler выводит текущие потоки либо проигрывает указанный
func (h *TmplHandlers) InfoHandler(c *gin.Context) {
	curr := h.items.GetTranslations()
	c.HTML(http.StatusOK, "info.html", curr)
}

// desk структура описатель для рендеринга
type desc struct {
	Keys    []localproxy.Key
	Stream  streamDesc
	Entries []zapcore.Entry
}

// LogHandler выводит детальную информацию о потоке
func (h *TmplHandlers) LogHandler(c *gin.Context) {
	path := c.Request.FormValue("path")
	if len(path) == 0 {
		c.HTML(http.StatusOK, "mess.html", Mess{Mess: "'path' is empty"})
	} else {
		res := desc{Keys: make([]localproxy.Key, 0)}
		if strings.HasSuffix(c.Request.URL.Path, "streamlog.html") {
			res.Keys = h.items.GetFiles(path)
			stream := h.stream.GetProcArgs(path)
			if stream != nil {
				res.Entries = stream.Log.Buffer(200)
				res.Stream.URL = stream.URLIn
				arr := strings.Split(stream.Name, "/")
				if len(arr) > 0 {
					res.Stream.User = arr[0]
				}
				if len(arr) > 1 {
					res.Stream.Cam = arr[1]
				}
				res.Stream.WorkDir = filepath.Join(stream.Dir, stream.Name)
				res.Stream.Notify = make([]notifyDesc, 0)
				for _, n := range stream.Notifications {
					res.Stream.Notify = append(res.Stream.Notify, notifyDesc{URL: n.URL, Key: n.Key, Value: n.Value})
				}
			} else {
				res.Entries = make([]zapcore.Entry, 0)
				res.Stream.Notify = make([]notifyDesc, 0)
			}
		} else if strings.HasSuffix(c.Request.URL.Path, "storagelog.html") {
			stream := h.storage.GetProcArgs(path)
			if stream != nil {
				res.Entries = stream.Log.Buffer(200)
				res.Stream.URL = stream.URLIn
				arr := strings.Split(stream.Name, "/")
				if len(arr) > 0 {
					res.Stream.User = arr[0]
				}
				if len(arr) > 1 {
					res.Stream.Cam = arr[1]
				}
				res.Stream.WorkDir = filepath.Join(stream.Dir, stream.Name)
				res.Stream.Notify = make([]notifyDesc, 0)
				for _, n := range stream.Notifications {
					res.Stream.Notify = append(res.Stream.Notify, notifyDesc{URL: n.URL, Key: n.Key, Value: n.Value})
				}
			} else {
				res.Entries = make([]zapcore.Entry, 0)
				res.Stream.Notify = make([]notifyDesc, 0)
			}
		} else {
			res.Entries = make([]zapcore.Entry, 0)
			res.Stream.Notify = make([]notifyDesc, 0)
		}

		c.HTML(http.StatusOK, "log.html", res)
	}
}

// HlsHandler выводит текущие потоки либо проигрывает указанный
func (h *TmplHandlers) HlsHandler(c *gin.Context) {
	url := c.Request.FormValue("url")
	if len(url) == 0 {
		curr := h.items.GetTranslations()
		c.HTML(http.StatusOK, "hls.html", curr)
		return
	}
	c.HTML(http.StatusOK, "hlsvideo.html", url)
}

// ShakaHandler выводит текущие потоки либо проигрывает указанный
func (h *TmplHandlers) ShakaHandler(c *gin.Context) {
	url := c.Request.FormValue("url")
	if len(url) == 0 {
		curr := h.items.GetTranslations()
		c.HTML(http.StatusOK, "shaka.html", curr)
		return
	}
	c.HTML(http.StatusOK, "shakavideo.html", url)
}

// DashHandler выводит текущие потоки либо проигрывает указанный
func (h *TmplHandlers) DashHandler(c *gin.Context) {
	url := c.Request.FormValue("url")
	if len(url) == 0 {
		curr := h.items.GetTranslations()
		c.HTML(http.StatusOK, "dash.html", curr)
		return
	}
	c.HTML(http.StatusOK, "dashvideo.html", url)
}
