package localproxy

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/antchfx/xmlquery"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"camctl/local/localconf"
	"camctl/local/localnotif"
)

// Item data struct in cache
type Item struct {
	data        []byte
	contentType string
	created     time.Time
	timeout     time.Duration
}

type itemCond struct {
	data *Item
	cond *sync.Cond
}

type delItem struct {
	key     string
	created time.Time
}

// Items file storage
type Items struct {
	wg            *sync.WaitGroup
	log           *zap.Logger
	conf          *localconf.Config
	items         map[string]*itemCond                  // тут хранятся данные: чанки и мета описатели - ключ / файл
	notifications map[string][]*localnotif.Notification // тут хранятся нотификации
	delPrefix     []delItem
	fileMut       *sync.RWMutex
	timeout       time.Duration // общий
	maxTimeout    time.Duration // для init сегментов, *.m3u8, *.mpd - они обязательны для mpeg-dash
	waitData      time.Duration // ожидание из кеша
	worked        *int32
}

// AddNotifications store notification servers into storage and bind it with name
func (f *Items) AddNotifications(name string, array localnotif.Notifications) {
	f.fileMut.Lock()
	defer f.fileMut.Unlock()
	f.notifications[name] = array
}

// GetNotifications return notification servers by bind name
func (f *Items) GetNotifications(name string) (localnotif.Notifications, bool) {
	f.fileMut.Lock()
	defer f.fileMut.Unlock()
	res, isFind := f.notifications[name]
	return res, isFind
}

// DelNotifications remove notification servers by bind name, return it if it finded
func (f *Items) DelNotifications(name string) (localnotif.Notifications, bool) {
	f.fileMut.Lock()
	defer f.fileMut.Unlock()
	res, isFind := f.notifications[name]
	if isFind {
		delete(f.notifications, name)
	}
	return res, isFind
}

func (f *Items) clean() {
	f.wg.Add(1)
	defer f.wg.Done()
	for atomic.LoadInt32(f.worked) != 0 {
		count := f.Clean()
		f.log.Sugar().Warnf("Items.Clean %d goroutines %d", count, runtime.NumGoroutine())
		time.Sleep(f.timeout / 10)
	}
}

// NewItems create Items
func NewItems(wg *sync.WaitGroup, logger *zap.Logger, config *localconf.Config, timeout time.Duration, maxtimeout time.Duration, waitdata time.Duration) *Items {
	res := &Items{wg, logger, config, make(map[string]*itemCond), make(map[string][]*localnotif.Notification), make([]delItem, 0, 1), new(sync.RWMutex), timeout, maxtimeout, waitdata, new(int32)}
	atomic.StoreInt32(res.worked, 1)
	go res.clean() // тут удаляются в том числе init-stream0.m4s и init-stream1.m4s без них js плеер падает. Ffmpeg сам удаляет старое вызывает DELETE
	return res
}

func (f *Items) Close() {
	atomic.StoreInt32(f.worked, 0)
}

func copyXML(source *xmlquery.Node, dest *xmlquery.Node) {
	dest.Type = source.Type
	dest.Data = source.Data
	dest.Prefix = source.Prefix
	dest.NamespaceURI = source.NamespaceURI
	if source.Attr != nil {
		dest.Attr = make([]xml.Attr, 0)
		for _, attr := range source.Attr {
			dest.Attr = append(dest.Attr, xml.Attr{Name: xml.Name{Space: attr.Name.Space, Local: attr.Name.Local}, Value: attr.Value})
		}
	}
	var prevNewChild *xmlquery.Node = nil
	currSource := source.FirstChild
	for currSource != nil {
		newDoc := new(xmlquery.Node)
		newDoc.PrevSibling = prevNewChild
		copyXML(currSource, newDoc)
		if prevNewChild == nil {
			dest.FirstChild = newDoc
		} else {
			prevNewChild.NextSibling = newDoc
		}
		prevNewChild = newDoc
		currSource = currSource.NextSibling
	}
	dest.LastChild = prevNewChild
}

func addAttr(node *xmlquery.Node, attr xml.Attr) {
	index := -1
	for i, curr := range node.Attr {
		if curr.Name.Local == attr.Name.Local {
			index = i
			break
		}
	}
	if index != -1 {
		//node.Attr = append(node.Attr[:index], node.Attr[index+1:]...)
		node.Attr[index].Value = attr.Value
	} else {
		node.Attr = append(node.Attr, attr)
	}
}

/*
add attributes to xml
*/
func xmlProcessing(data []byte) []byte {
	doc, err := xmlquery.Parse(bytes.NewReader(data))
	if err != nil {
		return data
	}
	mpd, errMpd := xmlquery.Query(doc, "//MPD")
	if errMpd == nil && mpd != nil {
		addAttr(mpd, xml.Attr{Name: xml.Name{Local: "minimumUpdatePeriod"}, Value: "PT30S"})
	}
	sd, errLatency := xmlquery.Query(doc, "//MPD/ServiceDescription")
	if errLatency == nil && sd != nil {
		// scope, errScope := xmlquery.Query(sd, "//Scope")
		// if errScope == nil {
		// 	if scope != nil {
		// 		scope.Attr = append(scope.Attr, xml.Attr{Name: xml.Name{Local: "schemeIdUri"}, Value: "urn:dvb:dash:lowlatency:scope:2019"})
		// 	} else {
		// 		sd.
		// 	}
		// }
		latency, errLatency := xmlquery.Query(sd, "//Latency")
		if errLatency == nil {
			if latency == nil {
				latency = new(xmlquery.Node)
				latency.Data = "Latency"
				latency.Type = xmlquery.ElementNode
				if sd.FirstChild == nil {
					sd.FirstChild = latency
					sd.LastChild = latency
				} else {
					latency.PrevSibling = sd.LastChild
					sd.LastChild.NextSibling = latency
					sd.LastChild = latency
				}
				latency.Parent = sd
			}
			addAttr(latency, xml.Attr{Name: xml.Name{Local: "target"}, Value: "2000"})
			addAttr(latency, xml.Attr{Name: xml.Name{Local: "min"}, Value: "1500"})
			addAttr(latency, xml.Attr{Name: xml.Name{Local: "max"}, Value: "3000"})
		}
	}
	return []byte(doc.OutputXML(false))
}

func fingKeyGetVal(array []string, key string) string {
	if !strings.HasSuffix(key, "=") {
		key = key + "="
	}
	for _, item := range array {
		if strings.HasPrefix(item, key) {
			return item[len(key):]
		}
	}
	return ""
}

func trimString(str string, preffix string, suffix string) string {
	if strings.HasPrefix(str, preffix) {
		str = str[len(preffix):]
	}
	if strings.HasSuffix(str, suffix) {
		str = str[:len(str)-len(suffix)]
	}
	return str
}

/*
добавляем атрибуты которые не генерит ffmpeg - просто audio не проигрывается в hls.js
*/
func hlsProcessing(data []byte) []byte {
	scaner := bufio.NewScanner(bytes.NewReader(data))
	extXMedia := ""
	extXStreamInf := ""
	var res bytes.Buffer
	for scaner.Scan() {
		str := scaner.Text()
		res.WriteString(str)
		res.WriteString("\n")
		if strings.HasPrefix(str, "#EXT-X-MEDIA:") {
			extXMedia = str
		} else if strings.HasPrefix(str, "#EXT-X-STREAM-INF:") {
			extXStreamInf = str
		}
	}
	if len(extXStreamInf) > 0 {
		return data
	}
	array := strings.Split(extXMedia[13:], ",")
	if fingKeyGetVal(array, "TYPE") != "AUDIO" {
		return data
	}
	groupID := trimString(fingKeyGetVal(array, "GROUP-ID"), "\"", "\"")
	uri := trimString(fingKeyGetVal(array, "URI"), "\"", "\"")
	res.WriteString("#EXT-X-STREAM-INF:BANDWIDTH=132056,CODECS=\"avc1.64001e\",AUDIO=\"")
	res.WriteString(groupID)
	res.WriteString("\"\n")
	res.WriteString(uri)
	return res.Bytes()
}

// MakeRequest make callback request
func MakeRequest(url string, data url.Values) ([]byte, error) {
	var resp *http.Response = nil
	var err error = nil
	if len(data) == 0 {
		resp, err = http.Get(url)
	} else {
		resp, err = http.PostForm(url, data)
	}
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err == nil {
		resp.Body.Close()
	}
	return body, err
}

func (f *Items) notify(url string, data url.Values) {
	defer func() {
		if r := recover(); r != nil {
			_, file, line, _ := runtime.Caller(0)
			f.log.Sugar().Errorf("notify Recovered path=%s line=%d error=%v", file, line, r)
		}
	}()
	MakeRequest(url, data)
}

// IsNumber return true is rune is number
func IsNumber(check rune) bool {
	switch check {
	case '0':
		return true
	case '1':
		return true
	case '2':
		return true
	case '3':
		return true
	case '4':
		return true
	case '5':
		return true
	case '6':
		return true
	case '7':
		return true
	case '8':
		return true
	case '9':
		return true
	default:
		return false
	}
}

/*
смотрим запуск ffmpeg
-init_seg_name init_name
DASH-templated name to used for the initialization segment.
Default is "init-stream$RepresentationID$.$ext$".
"$ext$" is replaced with the file name extension specific for the segment format.
*/
func parseChannel(key string) int {
	index := strings.Index(key, localconf.InitSegmentName)
	if index != -1 {
		str := key[index+len(localconf.InitSegmentName):]
		arr := []rune(str)
		begin := -1
		for i, r := range arr {
			if begin == -1 {
				if IsNumber(r) {
					begin = i
				}
			} else if begin != -1 {
				if !IsNumber(r) {
					number := string(arr[begin:i])
					res, err := strconv.Atoi(number)
					if err == nil {
						return res
					}
					return -1
				}
			}
		}
	}
	return -1
}

/*
смотрим шаблоны sdp
0 audio
1 video
*/
func channelName(number int) string {
	switch number {
	case 0:
		return "audio"
	case 1:
		return "video"
	default:
		return ""
	}
}

// Add store data in cache
func (f *Items) Add(key string, data []byte, contentType string) *Item {
	timeout := f.timeout
	channel := -1
	if strings.Index(key, localconf.InitSegmentName) != -1 {
		channel = parseChannel(key)
		timeout = f.maxTimeout
	} else if strings.HasSuffix(key, ".mpd") {
		timeout = f.maxTimeout
		data = xmlProcessing(data)
	} else if strings.HasSuffix(key, ".m3u8") {
		timeout = f.maxTimeout
		if strings.HasSuffix(key, "master.m3u8") {
			data = hlsProcessing(data)
		}
	}

	item := &Item{data: data, contentType: contentType, created: time.Now(), timeout: timeout}

	var res *Item = nil

	// блокировка мапы
	f.fileMut.Lock()
	find, isFind := f.items[key]
	if !isFind { // новый элемент
		// меняем содержимое мапы
		f.items[key] = &itemCond{data: item, cond: sync.NewCond(&sync.Mutex{})}
		// разблокируем мапу
		f.fileMut.Unlock()
	} else { // существующий элемент
		// разблокируем мапу
		f.fileMut.Unlock()
		// берем блокировку на содержимое
		find.cond.L.Lock()
		// меняем содержимое, запомним старое значение
		res = find.data
		find.data = item
		// нотификация возможных ожидающих
		find.cond.Broadcast()
		// отпускаем блокировку на содержимое
		find.cond.L.Unlock()
	}

	if channel != -1 {
		// TODO - исправить
		// go f.notify(*config.notifyURL, url.Values{"path": {key}, "channel": {channelName(channel)}})
	}

	return res
}

// Del delete data in cache
func (f *Items) Del(key string) *Item {
	f.fileMut.Lock()
	defer f.fileMut.Unlock()

	var res *Item = nil
	find, isFind := f.items[key]
	if isFind {
		// существующий элемент
		find.cond.L.Lock()
		res = find.data
		find.data = nil
		delete(f.items, key)
		find.cond.Broadcast()
		find.cond.L.Unlock()
	}
	return res
}

// Get Сложная функция если данные есть она их отдает если данных нет то она их ждет с таймаутом
// WaitDataInCache. Важно: время ожидания должно быть ОДИНАКОВЫМ для доступа из любых мест,
// иначе возможно получение пустых данных без гарантированного таймаута
func (f *Items) Get(key string) *Item {
	var res *Item = nil

	// блокировка мапы
	f.fileMut.Lock()
	find, isFind := f.items[key]
	if isFind { // элемент существует
		// разблокируем мапу
		f.fileMut.Unlock()
		// берем блокировку на содержимое
		find.cond.L.Lock()
		if find.data != nil { // данные есть
			res = find.data
		} else { // данных нет: такое будет если какой то поток тоже ожидает данные
			// сначала себя обезопасим если совсем никто писать в мапу не будет
			go func() {
				time.Sleep(f.waitData)
				f.wg.Add(1)
				defer f.wg.Done()
				find.cond.L.Lock()
				find.cond.Broadcast()
				find.cond.L.Unlock()
			}()
			// теперь ждем
			find.cond.Wait()
			// теперь читаем
			res = find.data
		}
		// отпускаем блокировку на содержимое
		find.cond.L.Unlock()
	} else { // элемента нет - создаем его ждем записи
		// меняем содержимое мапы
		item := &itemCond{data: nil, cond: sync.NewCond(&sync.Mutex{})}
		f.items[key] = item
		// разблокируем мапу
		f.fileMut.Unlock()
		// берем блокировку на содержимое
		item.cond.L.Lock()
		// сначала себя обезопасим если совсем никто писать в мапу не будет
		go func() {
			time.Sleep(f.waitData)
			f.wg.Add(1)
			defer f.wg.Done()
			item.cond.L.Lock()
			item.cond.Broadcast()
			item.cond.L.Unlock()
		}()
		// теперь ждем
		item.cond.Wait()
		// теперь читаем
		res = item.data
		// отпускаем блокировку на содержимое
		item.cond.L.Unlock()
		// если данных так и не появилось - убираем из мапины структуры мозданные функцией и связанные с ожиданием данных
		if res == nil {
			f.Del(key)
		}
	}
	return res
}

// Clean clean old data from cache
func (f *Items) Clean() int {

	// в случае завершения работы ждем окончания всех обработчиков
	// wg.Add(1)
	// defer wg.Done()

	res := 0
	keys := f.Keys()
	now := time.Now()
	pass := make([]delItem, 0, 1)
	for _, prefix := range f.delPrefix {
		if now.Sub(prefix.created) <= f.timeout {
			pass = append(pass, prefix)
		}
	}
	for _, key := range keys {
		item := f.Get(key)
		if item == nil {
			// элемен удален вызовом Get
			continue
		} else if now.Sub(item.created) > item.timeout {
			f.Del(key)
			f.log.Sugar().Warnf("Items.Clean key %s", key)
			res++
		} else {
			for _, prefix := range f.delPrefix {
				if now.Sub(prefix.created) > f.timeout {
					if strings.HasPrefix(key, prefix.key) {
						f.Del(key)
						f.log.Sugar().Warnf("Items.Clean key %s", key)
						res++
					}
				}
			}
		}
	}
	f.delPrefix = pass
	return res
}

// Keys return keys stored in cache
func (f *Items) Keys() []string {
	f.fileMut.Lock()
	defer f.fileMut.Unlock()

	keys := make([]string, 0, len(f.items))
	for k := range f.items {
		keys = append(keys, k)
	}

	return keys
}

// Key struct for response
type Key struct {
	Key     string    `json:"key,omitempty"`
	Created time.Time `json:"created,omitempty"`
}

// KeysCreated build keys from cache
func (f *Items) KeysCreated() []Key {
	f.fileMut.Lock()
	defer f.fileMut.Unlock()

	keys := make([]Key, 0, len(f.items))
	for k, v := range f.items {
		keys = append(keys, Key{Key: k, Created: v.data.created})
	}

	return keys
}

// DelAny - delete all keys start with key in next iteration
func (f *Items) DelAny(key string) {
	f.fileMut.Lock()
	defer f.fileMut.Unlock()

	if len(key) > 0 {
		f.log.Sugar().Warnf("DelAny for %s", key)
		f.delPrefix = append(f.delPrefix, delItem{key, time.Now()})
	}
}

// CancelDelAny cancel delete all keys start with key in next iteration
func (f *Items) CancelDelAny(key string) int {
	f.fileMut.Lock()
	defer f.fileMut.Unlock()

	if len(key) == 0 {
		return 0
	}
	f.log.Sugar().Warnf("CancelDelAny for %s", key)

	deleted := 0
	for i := range f.delPrefix {
		j := i - deleted
		if strings.HasPrefix(key, f.delPrefix[j].key) {
			f.delPrefix = f.delPrefix[:j+copy(f.delPrefix[j:], f.delPrefix[j+1:])]
			deleted++
		}
	}

	return deleted
}

// GetTranslations return translations from cache
func (f *Items) GetTranslations() map[string]int {
	keys := f.Keys()
	res := make(map[string]int)
	for _, key := range keys {
		paths := strings.Split(key, "/")
		path := ""
		for i := 0; i < len(paths)-1; i++ {
			path += paths[i]
			if i < len(paths)-2 {
				path += "/"
			}
		}
		numbers, _ := res[path]
		res[path] = numbers + 1
	}
	return res
}

// GetFiles return files for translation
func (f *Items) GetFiles(path string) []Key {
	keys := f.KeysCreated()
	res := make([]Key, 0)
	for _, key := range keys {
		if strings.HasPrefix(key.Key, path) {
			res = append(res, key)
		}
	}
	return res
}

// Error helper for fast http out
func Error(c *gin.Context, mess string, code int) {
	c.Data(code, "text/plain; charset=utf-8", []byte(mess))
}

// func (f *Items) ServeHTTP(w http.ResponseWriter, r *http.Request) {
func (f *Items) ServeHTTP(c *gin.Context) {
	now := time.Now()
	f.log.Info("request", zap.Time("Time", now), zap.String("URL", c.Request.URL.String()), zap.String("Method", c.Request.Method), zap.String("ContentType", c.Request.Header.Get("Content-Type")))

	if strings.HasPrefix(c.Request.URL.Path, "/put") {

		// проверка на ip
		if !f.conf.IsTrustedIP(c.Request.RemoteAddr) {
			f.log.Sugar().Warnf("forbidden by remote ip %s", c.Request.RemoteAddr)
			Error(c, "forbidden", http.StatusForbidden)
			return
		}

		if c.Request.Method == "PUT" {
			key := c.Request.URL.Path[4:]
			body, err := ioutil.ReadAll(c.Request.Body)
			if err != nil {
				f.log.Sugar().Warnf("Error reading body: %v, key %s", err, key)
				Error(c, "can't read body", http.StatusNoContent)
			} else {
				c.Request.Body.Close()
				res := f.Add(key, body, c.Request.Header.Get("Content-Type"))
				if res == nil {
					f.log.Sugar().Infof("Create by key: %v, len(body): %d", key, len(body))
					Error(c, "created", http.StatusCreated)
				} else {
					f.log.Sugar().Infof("Update by key: %v, len(body): %d", key, len(body))
					Error(c, "update", http.StatusOK)
				}
				array, isFind := f.GetNotifications(filepath.Dir(key))
				if isFind && array != nil {
					array.Send(f.log, &localnotif.NotificationData{Method: "PUT", Name: filepath.Base(key), Header: c.Request.Header, Data: body})
				}
			}
		} else if c.Request.Method == "DELETE" {
			key := c.Request.URL.Path[4:]
			if strings.Index(key, ".") == -1 {
				f.DelAny(key)
				f.log.Sugar().Infof(fmt.Sprintf("Delete by key: %v is accepted", key))
				Error(c, "accepted", http.StatusAccepted)
			} else {
				res := f.Del(key)
				if res == nil {
					f.log.Sugar().Infof("Delete by key: %v not found", key)
					Error(c, "created", http.StatusNoContent)
				} else {
					f.log.Sugar().Infof("Delete by key: %v, deleted\n", key)
					Error(c, "update", http.StatusOK)
				}
			}
			array, isFind := f.GetNotifications(filepath.Dir(key))
			if isFind && array != nil {
				array.Send(f.log, &localnotif.NotificationData{Method: "DELETE", Name: filepath.Base(key), Header: c.Request.Header, Data: nil})
			}
		}
	} else if strings.HasPrefix(c.Request.URL.Path, "/get") {
		key := c.Request.URL.Path[4:]
		res := f.Get(key)
		if res == nil {
			Error(c, "no content", http.StatusNoContent)
		} else {
			headers := make(map[string]string)
			headers["Date"] = res.created.UTC().Format(http.TimeFormat)
			c.DataFromReader(http.StatusOK, int64(len(res.data)), "", bytes.NewReader(res.data), headers)
		}
	} else if strings.HasPrefix(c.Request.URL.Path, "/info") {
		key := c.Request.URL.Path[5:]
		if len(key) == 0 {
			res := f.GetTranslations()
			c.JSON(http.StatusOK, Response{Errno: OK, Error: "ok", Data: res})
		} else {
			res := f.GetFiles(key)
			c.JSON(http.StatusOK, Response{Errno: OK, Error: "ok", Data: res})
		}
	}
}
