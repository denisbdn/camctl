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
	"camctl/local/localproxy"
)

const (
	// StorageFfmpegCmd - command for ffmpeg execute
	// StorageFfmpegCmd       string = "ffmpeggpu.cmd"
	StorageFfmpegCmd string = "repackffmpegfs.cmd"
)

// StorageHandler describe http handler object
type StorageHandler struct {
	log         *zap.Logger
	conf        *localconf.Config
	procArgs    map[string]*StorageFFMPEG
	procArgsMut *sync.RWMutex
}

// NewStorageHandler create http handler
func NewStorageHandler(logger *zap.Logger, config *localconf.Config) *StorageHandler {
	res := StorageHandler{log: logger, conf: config, procArgs: make(map[string]*StorageFFMPEG), procArgsMut: new(sync.RWMutex)}
	return &res
}

func (h *StorageHandler) setProcArgs(proc string, procArgs *StorageFFMPEG) *StorageFFMPEG {
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
func (h *StorageHandler) GetProcArgs(proc string) *StorageFFMPEG {
	h.procArgsMut.Lock()
	defer h.procArgsMut.Unlock()
	find, isFind := h.procArgs[proc]
	if !isFind {
		return nil
	}
	return find
}

// GetProcArgs return stored *FFMPEG object by key
func (h *StorageHandler) GetProcArgsFFMPEG(proc string) *FFMPEG {
	h.procArgsMut.Lock()
	defer h.procArgsMut.Unlock()
	find, isFind := h.procArgs[proc]
	if !isFind {
		return nil
	}
	return &find.FFMPEG
}

func (h *StorageHandler) delProcArgs(proc string) *StorageFFMPEG {
	h.procArgsMut.Lock()
	defer h.procArgsMut.Unlock()
	find, isFind := h.procArgs[proc]
	if !isFind {
		return nil
	}
	delete(h.procArgs, proc)
	return find
}

func (h *StorageHandler) delEmptyDir(path string) error {
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

/*
-master_pl_name master.m3u8 опция игнорируется ffmpeg можно написать master_pl_name out.m3u8 но генерироваться будет master.m3u8
*/
func (h *StorageHandler) runFFMPEG(txtPath string, argsStr string, procArgs *StorageFFMPEG) {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.StampNano)
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	logger, _ := cfg.Build()
	procArgs.Log = locallog.NewBuffLog(logger, 250)
	defer procArgs.Log.Close()

	h.log.Sugar().Warnf("start runFFMPEG for %s", txtPath)
	procArgs.Log.Log.Sugar().Warnf("start runFFMPEG for %s", txtPath)

	for _, n := range procArgs.Notifications {
		go n.Notify(procArgs.Log.Log)
	}

	h.setProcArgs("/"+procArgs.Name, procArgs)

	args := SplitArgs(argsStr)
	h.log.Sugar().Warnf(fmt.Sprintf("%s %v", "ffmpeg", args))
	procArgs.Log.Log.Sugar().Warnf(fmt.Sprintf("%s %v", "ffmpeg", args))
	cmd := exec.Command("ffmpeg", args...)
	logFile, errFile := os.Create(txtPath + ".log")
	if errFile != nil {
		h.log.Sugar().Errorf("os.Create() for %s.log return error: %s", txtPath, errFile.Error())
		procArgs.Log.Log.Sugar().Errorf("os.Create() for %s.log return error: %s", txtPath, errFile.Error())
		return
	}
	defer os.Remove(txtPath + ".log")
	defer h.delEmptyDir(filepath.Dir(txtPath))
	stderr, err := cmd.StderrPipe()
	if err != nil {
		h.log.Sugar().Errorf("cmd.StderrPipe() for %s return error: %s", txtPath, err.Error())
		procArgs.Log.Log.Sugar().Errorf("cmd.StderrPipe() for %s return error: %s", txtPath, err.Error())
		return
	}
	defer stderr.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		h.log.Sugar().Errorf("cmd.StderrPipe() for %s return error: %s", txtPath, err.Error())
		procArgs.Log.Log.Sugar().Errorf("cmd.StderrPipe() for %s return error: %s", txtPath, err.Error())
		return
	}
	defer stdout.Close()

	go func() {
		procArgs.Log.Log.Sugar().Warnf("start cmd.Run() for %s", txtPath)
		errRun := cmd.Run()
		if errRun != nil {
			procArgs.Log.Log.Sugar().Errorf("stop cmd.Run() for %s return error: %s", txtPath, errRun.Error())
			time.Sleep(time.Millisecond * 200)
			os.Remove(txtPath)
			return
		}
		procArgs.Log.Log.Sugar().Warnf("stop cmd.Run() for %s", txtPath)
		os.Remove(txtPath)
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
			procArgs.Log.Log.Sugar().Warnf("start read err channel for %s", txtPath)
			scannerErr := bufio.NewScanner(stderr)
			for scannerErr.Scan() {
				atomicWriteSync("FFMPEG error stream: ", scannerErr.Text()) // Println will add back the final '\n'
			}
			procArgs.Log.Log.Sugar().Warnf("stop read err channel for %s", txtPath)
			defer h.delEmptyDir(filepath.Dir(txtPath))
		}()
		go func() {
			procArgs.Log.Log.Sugar().Warnf("start read out channel for %s", txtPath)
			scannerOut := bufio.NewScanner(stdout)
			for scannerOut.Scan() {
				atomicWrite("FFMPEG out stream: ", scannerOut.Text()) // Println will add back the final '\n'
			}
			procArgs.Log.Log.Sugar().Warnf("stop read out channel for %s", txtPath)
			defer h.delEmptyDir(filepath.Dir(txtPath))
		}()
	}

	for {
		time.Sleep(time.Millisecond * 200)
		check, errOpen := os.Open(txtPath)
		check.Close()
		if errOpen != nil {
			break
		}
	}

	errSig := cmd.Process.Signal(syscall.SIGQUIT)
	if errSig != nil {
		h.log.Sugar().Warnf("Process.Signal %s", errSig.Error())
	}

	h.delProcArgs("/" + procArgs.Name)
	h.log.Sugar().Warnf("stop runFFMPEG for %s", txtPath)
	procArgs.Log.Log.Sugar().Warnf("stop runFFMPEG for %s", txtPath)
}

func (h *StorageHandler) start(c *gin.Context) {

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
	storeDir, pathError := filepath.Abs(filepath.Join(*h.conf.StoreDir, dir))
	if pathError != nil {
		localproxy.Error(c, "Unable build path to file ", http.StatusInternalServerError)
		return
	}
	dirError := os.MkdirAll(storeDir, os.ModePerm)
	if dirError != nil {
		localproxy.Error(c, "error create dir", http.StatusBadRequest)
		return
	}

	// создаем файл sdp
	txtPath, pathError := filepath.Abs(filepath.Join(*h.conf.StoreDir, name+".txt"))
	if pathError != nil {
		localproxy.Error(c, "Unable build path to file ", http.StatusInternalServerError)
		return
	}

	file, errCreate := os.Create(txtPath)
	if errCreate != nil {
		localproxy.Error(c, "Unable to create file "+txtPath, http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// ищем шаблон для команды и аргументы
	tmplName := StorageFfmpegCmd
	procArgs := BuildStorageFFMPEG(name, storeDir, url, storeDir+name[dirEnd:], *h.conf.ChankDur, *h.conf.Chanks, c.Request.URL.Query()["notify"], c.Request.URL.Query()["onstart"], c.Request.URL.Query()["onstop"], c.Request.URL.Query()["onerror"])
	tmpl, ok := h.conf.GetTmpl(tmplName)
	if !ok {
		localproxy.Error(c, tmplName+" not found", http.StatusInternalServerError)
		os.Remove(storeDir)
		return
	}
	// строим команду запуска
	buf := bytes.NewBufferString("")
	errTmpl := tmpl.Execute(buf, *procArgs)
	if errTmpl != nil {
		h.log.Error("build command", zap.Error(errTmpl))
		localproxy.Error(c, tmplName+" not build", http.StatusInternalServerError)
		os.Remove(storeDir)
		return
	}

	// сначала ответ
	localproxy.Error(c, "created", http.StatusCreated)

	go h.runFFMPEG(txtPath, buf.String(), procArgs)
}

func (h *StorageHandler) stop(c *gin.Context) {
	path := c.Request.URL.Path
	nameBegin := strings.LastIndex(path, "/stop/")
	name := path[nameBegin+6:]
	if len(name) == 0 {
		localproxy.Error(c, "name isn't set in path", http.StatusBadRequest)
		return
	}

	// создаем файл sdp
	sdpPath, pathError := filepath.Abs(filepath.Join(*h.conf.StoreDir, name+".txt"))
	if pathError != nil {
		localproxy.Error(c, "Unable build path to file ", http.StatusInternalServerError)
		return
	}

	// удаляем файл
	os.Remove(sdpPath)

	localproxy.Error(c, "deleted", http.StatusAccepted)
}

func (h *StorageHandler) ServeHTTP(c *gin.Context) {
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
