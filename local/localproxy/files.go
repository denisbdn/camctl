package localproxy

import (
	"camctl/local/localconf"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Items file storage
type Files struct {
	wg      *sync.WaitGroup
	log     *zap.Logger
	conf    *localconf.Config
	timeout time.Duration // общий
	worked  *int32
}

func cleanFS(dir string, timeout time.Duration) (int, error) {
	files, errReadDir := ioutil.ReadDir(dir)
	if errReadDir != nil {
		return 0, errReadDir
	}
	count := 0
	check := time.Now().Add(-1 * timeout)
	for _, f := range files {
		if f.IsDir() {
			cnt, err := cleanFS(dir+"/"+f.Name(), timeout)
			count = count + cnt
			if err != nil {
				return count, err
			}
		} else {
			if f.ModTime().Before(check) {
				errRemove := os.Remove(dir + "/" + f.Name())
				if errRemove != nil {
					return count, errRemove
				}
				count = count + 1
			}
		}
	}
	return count, nil
}

func (f *Files) clean() {
	f.wg.Add(1)
	defer f.wg.Done()
	for atomic.LoadInt32(f.worked) != 0 {
		count, err := cleanFS(*f.conf.StoreDir, f.timeout)
		if count != 0 {
			f.log.Sugar().Warnf("Files.Clean %d\n", count)
		}
		if err != nil {
			f.log.Sugar().Errorf("Files.Clean error %s\n", err.Error())
		}
		time.Sleep(f.timeout / 10)
	}
}

// NewItems create Items
func NewFiles(wg *sync.WaitGroup, logger *zap.Logger, config *localconf.Config, timeout time.Duration) *Files {
	res := &Files{wg, logger, config, timeout, new(int32)}
	atomic.StoreInt32(res.worked, 1)
	go res.clean() // тут удаляются старые файлы
	return res
}

func (f *Files) Close() {
	atomic.StoreInt32(f.worked, 0)
}

func infoFS(dir string, base string) []string {
	files, errReadDir := ioutil.ReadDir(dir)
	if errReadDir != nil {
		return nil
	}
	res := make([]string, 0)
	for _, f := range files {
		if f.IsDir() {
			array := infoFS(dir+"/"+f.Name(), base)
			if array != nil {
				res = append(res, array...)
			}
		} else {
			path := dir + "/" + f.Name()
			strings.Replace(path, base, "", 1)
			res = append(res, path)
		}
	}
	return res
}

func (h *Files) ServeHTTP(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/allhistory") {
		key := c.Request.URL.Path[11:]
		res := infoFS(*h.conf.StoreDir+key, *h.conf.StoreDir)
		if res == nil {
			c.JSON(http.StatusNotFound, Response{Errno: NotFound, Error: "directory not found"})
		} else {
			c.JSON(http.StatusOK, Response{Errno: OK, Error: "ok", Data: res})
		}
	}
}
