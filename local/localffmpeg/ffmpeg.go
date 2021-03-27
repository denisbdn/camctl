package localffmpeg

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"camctl/local/localconf"
	"camctl/local/locallog"
	"camctl/local/localnotif"
	"camctl/local/localproxy"
)

const (
	// FfmpegCmd - command for ffmpeg execute
	// FfmpegCmd       string = "ffmpeggpu.cmd"
	FfmpegCmd string = "ffmpeg.cmd"
)

// FFMPEG describe cache object
type FFMPEG struct {
	Name          string                     `json:"name,omitempty"`
	Dir           string                     `json:"dir,omitempty"`
	URLIn         string                     `json:"url,omitempty"`
	Port          uint                       `json:"-"`
	InitSegment   string                     `json:"-"`
	TimeStr       string                     `json:"time,omitempty"`
	Notifications []*localnotif.Notification `json:"notification,omitempty"`
	Log           *locallog.BuffLog          `json:"-"`
}

// FfmpegHandler describe http handler object
type FfmpegHandler struct {
	log         *zap.Logger
	conf        *localconf.Config
	items       *localproxy.Items
	procArgs    map[string]*FFMPEG
	procArgsMut *sync.RWMutex
}

// NewFfmpegHandler create http handler
func NewFfmpegHandler(logger *zap.Logger, config *localconf.Config, items *localproxy.Items) *FfmpegHandler {
	res := FfmpegHandler{log: logger, conf: config, items: items, procArgs: make(map[string]*FFMPEG), procArgsMut: new(sync.RWMutex)}
	return &res
}

func (h *FfmpegHandler) setProcArgs(proc string, procArgs *FFMPEG) *FFMPEG {
	h.procArgsMut.Lock()
	defer h.procArgsMut.Unlock()
	find, isFind := h.procArgs[proc]
	if procArgs != nil {
		h.procArgs[proc] = procArgs
	}
	if !isFind {
		return nil
	}
	return find
}

// GetProcArgs return stored *FFMPEG object by key
func (h *FfmpegHandler) GetProcArgs(proc string) *FFMPEG {
	h.procArgsMut.Lock()
	defer h.procArgsMut.Unlock()
	find, isFind := h.procArgs[proc]
	if !isFind {
		return nil
	}
	return find
}

func (h *FfmpegHandler) delProcArgs(proc string) *FFMPEG {
	h.procArgsMut.Lock()
	defer h.procArgsMut.Unlock()
	find, isFind := h.procArgs[proc]
	if !isFind {
		return nil
	}
	delete(h.procArgs, proc)
	return find
}

func (h *FfmpegHandler) delEmptyDir(path string) error {
	files, errRead := ioutil.ReadDir(path)
	if errRead != nil {
		return errRead
	}

	if len(files) != 0 {
		h.log.Sugar().Infof("directiry %s has %d", path, len(files))
		for _, fi := range files {
			h.log.Sugar().Infof("file %s", fi.Name())
		}
		return fmt.Errorf("directory %s isn't empty", path)
	}

	return os.Remove(path)
}

// SplitArgs каждый специальный аргумент должен быть заключен в кавычки, а кавычки не должны граничить с пробелами внутри аргумента
// "id=0,streams=v id=1,streams=a" - хорошо
// "id=0,streams=v id=1,streams=a " - плохо
// " id=0,streams=v id=1,streams=a" - плохо
func SplitArgs(argsStr string) []string {
	firstSplit := strings.Split(argsStr, "\"")
	array := make([]string, 0)
	for _, item := range firstSplit {
		if strings.HasPrefix(item, " ") {
			add := strings.Split(item[1:], " ")
			array = append(array, add...)
		} else if strings.HasSuffix(item, " ") {
			add := strings.Split(item[0:len(item)-1], " ")
			array = append(array, add...)
		} else {
			array = append(array, item)
		}
	}
	return array
}

/*
-master_pl_name master.m3u8 опция игнорируется ffmpeg можно написать master_pl_name out.m3u8 но генерироваться будет master.m3u8
*/
func (h *FfmpegHandler) runFFMPEG(sdpPath string, argsStr string, procArgs *FFMPEG) {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.StampNano)
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	logger, _ := cfg.Build()
	procArgs.Log = locallog.NewBuffLog(logger, 250)
	defer procArgs.Log.Close()

	h.log.Sugar().Warnf("start runFFMPEG for %s", sdpPath)
	procArgs.Log.Log.Sugar().Warnf("start runFFMPEG for %s", sdpPath)

	for _, n := range procArgs.Notifications {
		go n.Notify(procArgs.Log.Log)
	}

	var key string
	wd, errAbs := filepath.Abs(*h.conf.WorkDir)
	if errAbs == nil {
		key = strings.Replace(sdpPath, wd, "", 1)
		ext := filepath.Ext(key)
		if len(ext) > 0 {
			key = strings.Replace(key, ext, "", 1)
		}
		h.items.AddNotifications(key, procArgs.Notifications)
	}

	h.setProcArgs("/"+procArgs.Name, procArgs)

	args := SplitArgs(argsStr)
	h.log.Sugar().Warnf(fmt.Sprintf("%s %v", "ffmpeg", args))
	procArgs.Log.Log.Sugar().Warnf(fmt.Sprintf("%s %v", "ffmpeg", args))
	cmd := exec.Command("ffmpeg", args...)
	logFile, errFile := os.Create(sdpPath + ".log")
	if errFile != nil {
		h.log.Sugar().Errorf("os.Create() for %s.log return error: %s", sdpPath, errFile.Error())
		procArgs.Log.Log.Sugar().Errorf("os.Create() for %s.log return error: %s", sdpPath, errFile.Error())
		return
	}
	defer os.Remove(sdpPath + ".log")
	defer h.delEmptyDir(filepath.Dir(sdpPath))
	stderr, err := cmd.StderrPipe()
	if err != nil {
		h.log.Sugar().Errorf("cmd.StderrPipe() for %s return error: %s", sdpPath, err.Error())
		procArgs.Log.Log.Sugar().Errorf("cmd.StderrPipe() for %s return error: %s", sdpPath, err.Error())
		return
	}
	defer stderr.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		h.log.Sugar().Errorf("cmd.StderrPipe() for %s return error: %s", sdpPath, err.Error())
		procArgs.Log.Log.Sugar().Errorf("cmd.StderrPipe() for %s return error: %s", sdpPath, err.Error())
		return
	}
	defer stdout.Close()

	go func() {
		procArgs.Log.Log.Sugar().Warnf("start cmd.Run() for %s", sdpPath)
		errRun := cmd.Run()
		if errRun != nil {
			procArgs.Log.Log.Sugar().Errorf("stop cmd.Run() for %s return error: %s", sdpPath, errRun.Error())
			time.Sleep(time.Millisecond * 200)
			return
		}
		procArgs.Log.Log.Sugar().Warnf("stop cmd.Run() for %s", sdpPath)
	}()

	{
		mutex := sync.Mutex{}
		atomicWrite := func(text ...string) {
			mutex.Lock()
			defer mutex.Unlock()
			var sb strings.Builder
			for _, str := range text {
				sb.WriteString(str)
			}
			logFile.WriteString(sb.String())
			logFile.WriteString("\n")
			procArgs.Log.Log.Sugar().Info(sb.String())
			sb.Reset()
		}
		atomicWriteSync := func(text ...string) {
			mutex.Lock()
			defer mutex.Unlock()
			var sb strings.Builder
			for _, str := range text {
				sb.WriteString(str)
			}
			logFile.WriteString(sb.String())
			logFile.WriteString("\n")
			logFile.Sync()
			procArgs.Log.Log.Sugar().Info(sb.String())
			sb.Reset()
		}
		atomicWriteSync("ffmpeg ", argsStr)
		go func() {
			procArgs.Log.Log.Sugar().Warnf("start read err channel for %s", sdpPath)
			scannerErr := bufio.NewScanner(stderr)
			for scannerErr.Scan() {
				atomicWriteSync("FFMPEG error stream: ", scannerErr.Text()) // Println will add back the final '\n'
			}
			procArgs.Log.Log.Sugar().Warnf("stop read err channel for %s", sdpPath)
			defer h.delEmptyDir(filepath.Dir(sdpPath))
		}()
		go func() {
			procArgs.Log.Log.Sugar().Warnf("start read out channel for %s", sdpPath)
			scannerOut := bufio.NewScanner(stdout)
			for scannerOut.Scan() {
				atomicWrite("FFMPEG out stream: ", scannerOut.Text()) // Println will add back the final '\n'
			}
			procArgs.Log.Log.Sugar().Warnf("stop read out channel for %s", sdpPath)
			defer h.delEmptyDir(filepath.Dir(sdpPath))
		}()
	}

	for true {
		time.Sleep(time.Millisecond * 200)
		check, errOpen := os.Open(sdpPath)
		check.Close()
		if errOpen != nil {
			break
		}
	}

	cmd.Process.Signal(syscall.SIGQUIT)
	if errAbs == nil {
		delNotifications, isFind := h.items.DelNotifications(key)
		if isFind && delNotifications != nil {
			delNotifications.Send(procArgs.Log.Log, &localnotif.NotificationData{Method: "DELETE", Name: "", Header: make(http.Header), Data: nil})
			delNotifications.Close()
		}
	}

	h.delProcArgs("/" + procArgs.Name)
	h.log.Sugar().Warnf("stop runFFMPEG for %s", sdpPath)
	procArgs.Log.Log.Sugar().Warnf("stop runFFMPEG for %s", sdpPath)
}

func buildFFMPEG(name string, Dir string, URLIn string, port uint, initSegment string, notifications []string) *FFMPEG {
	now := float64(time.Now().UTC().UnixNano()) / 1000000000
	nowStr := fmt.Sprintf("%.6f", now)
	nt := make([]*localnotif.Notification, 0)
	for _, str := range notifications {
		array := strings.Split(str, "|")
		if len(array) == 3 {
			nt = append(nt, &localnotif.Notification{URL: array[2], Key: array[0], Value: array[1], Channel: make(chan *localnotif.NotificationData, 30)})
		} else if len(array) == 2 {
			nt = append(nt, &localnotif.Notification{URL: array[1], Key: array[0], Channel: make(chan *localnotif.NotificationData, 30)})
		} else if len(array) == 1 {
			nt = append(nt, &localnotif.Notification{URL: array[0], Channel: make(chan *localnotif.NotificationData, 30)})
		}
	}
	data := FFMPEG{Name: name, Dir: Dir, URLIn: URLIn, Port: port, InitSegment: initSegment, TimeStr: nowStr, Notifications: nt}
	return &data
}

func (h *FfmpegHandler) start(c *gin.Context) {

	url := c.Request.URL.Query().Get("url")
	if len(url) == 0 {
		localproxy.Error(c, "url isn't set in query", http.StatusBadRequest)
		return
	}

	path := c.Request.URL.Path
	nameBegin := strings.LastIndex(path, "/start/")
	name := path[nameBegin+7:]
	if len(name) == 0 {
		localproxy.Error(c, "name isn't set in path", http.StatusBadRequest)
		return
	}

	dirEnd := strings.LastIndex(name, "/")
	if dirEnd == -1 {
		localproxy.Error(c, "must bee '/start/path1/path2'", http.StatusBadRequest)
		return
	}
	dir := name[0:dirEnd]

	// создаем каталог
	workDir, pathError := filepath.Abs(filepath.Join(*h.conf.WorkDir, dir))
	if pathError != nil {
		localproxy.Error(c, "Unable build path to file ", http.StatusInternalServerError)
		return
	}
	dirError := os.MkdirAll(workDir, os.ModePerm)
	if dirError != nil {
		localproxy.Error(c, "error create dir", http.StatusBadRequest)
		return
	}

	// создаем файл sdp
	sdpPath, pathError := filepath.Abs(filepath.Join(*h.conf.WorkDir, name+".sdp"))
	if pathError != nil {
		localproxy.Error(c, "Unable build path to file ", http.StatusInternalServerError)
		return
	}

	file, errCreate := os.Create(sdpPath)
	if errCreate != nil {
		localproxy.Error(c, "Unable to create file "+sdpPath, http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// ищем шаблон для команды и аргументы
	tmplName := FfmpegCmd
	procArgs := buildFFMPEG(name, workDir, url, *h.conf.Port, localconf.InitSegmentName, c.Request.URL.Query()["notify"])
	tmpl, ok := h.conf.GetTmpl(tmplName)
	if !ok {
		localproxy.Error(c, "ffmpeg.cmd not found", http.StatusInternalServerError)
		os.Remove(workDir)
		return
	}
	// строим команду запуска
	buf := bytes.NewBufferString("")
	errTmpl := tmpl.Execute(buf, *procArgs)
	if errTmpl != nil {
		h.log.Error("build command", zap.Error(errTmpl))
		localproxy.Error(c, "ffmpeg.cmd not build", http.StatusInternalServerError)
		os.Remove(workDir)
		return
	}

	// сначала ответ
	localproxy.Error(c, "created", http.StatusCreated)

	h.items.CancelDelAny("/" + name)
	go h.runFFMPEG(sdpPath, buf.String(), procArgs)

	return
}

func (h *FfmpegHandler) stop(c *gin.Context) {
	path := c.Request.URL.Path
	nameBegin := strings.LastIndex(path, "/stop/")
	name := path[nameBegin+6:]
	if len(name) == 0 {
		localproxy.Error(c, "name isn't set in path", http.StatusBadRequest)
		return
	}

	// создаем файл sdp
	sdpPath, pathError := filepath.Abs(filepath.Join(*h.conf.WorkDir, name+".sdp"))
	if pathError != nil {
		localproxy.Error(c, "Unable build path to file ", http.StatusInternalServerError)
		return
	}

	// удаляем файл
	os.Remove(sdpPath)

	// удаляем все связанное с трансляцией
	h.items.DelAny("/" + name) // после items.timeout, есть время на создание

	localproxy.Error(c, "deleted", http.StatusAccepted)
	return
}

func (h *FfmpegHandler) ServeHTTP(c *gin.Context) {
	// проверка на ip
	if !h.conf.IsTrustedIP(c.Request.RemoteAddr) {
		h.log.Sugar().Errorf("forbidden by remote ip %s", c.Request.RemoteAddr)
		localproxy.Error(c, "forbidden", http.StatusForbidden)
		return
	}
	if strings.Contains(c.Request.URL.Path, "/start") {
		h.start(c)
	} else if strings.Contains(c.Request.URL.Path, "/stop") {
		h.stop(c)
	} else {
		localproxy.Error(c, "bad path", http.StatusBadRequest)
		return
	}
}
