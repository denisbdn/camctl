package localffmpeg

import (
	"camctl/local/locallog"
	"camctl/local/localnotif"
	"fmt"
	"strings"
	"time"
)

// FFMPEG describe cache object
type FFMPEG struct {
	Name          string                     `json:"name,omitempty"`
	Dir           string                     `json:"dir,omitempty"`
	TimeStr       string                     `json:"time,omitempty"`
	Notifications []*localnotif.Notification `json:"notification,omitempty"`
	Log           *locallog.BuffLog          `json:"-"`
}

// StreamFFMPEG describe cache object
type StreamFFMPEG struct {
	FFMPEG
	URLIn       string `json:"urlin,omitempty"`
	Port        uint   `json:"port,omitempty"`
	InitSegment string `json:"init,omitempty"`
}

// StorageFFMPEG describe cache object
type StorageFFMPEG struct {
	FFMPEG
	URLIn         string `json:"urlin,omitempty"`
	URLOut        string `json:"urlout,omitempty"`
	ChankDuration uint   `json:"duration,omitempty"`
	StorageChanks uint   `json:"numbers,omitempty"`
}

func BuildStreamFFMPEG(name string, workDir string, URLIn string, port uint, initSegment string, notifications []string) *StreamFFMPEG {
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
	data := StreamFFMPEG{URLIn: URLIn, Port: port, InitSegment: initSegment, FFMPEG: FFMPEG{Name: name, Dir: workDir, TimeStr: nowStr, Notifications: nt}}
	return &data
}

func BuildStorageFFMPEG(name string, workDir string, URLIn string, URLOut string, ChankDuration uint, StorageChanks uint, notifications []string) *StorageFFMPEG {
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
	data := StorageFFMPEG{URLIn: URLIn, URLOut: URLOut, ChankDuration: ChankDuration, StorageChanks: StorageChanks, FFMPEG: FFMPEG{Name: name, Dir: workDir, TimeStr: nowStr, Notifications: nt}}
	return &data
}

// SplitArgs каждый специальный аргумент должен быть заключен в кавычки, а кавычки не должны граничить с пробелами внутри аргумента
// "id=0,streams=v id=1,streams=a" - хорошо
// "id=0,streams=v id=1,streams=a " - плохо
// " id=0,streams=v id=1,streams=a" - плохо
func SplitArgs(argsStr string) []string {
	firstSplit := strings.Split(argsStr, "\"")
	array := make([]string, 0)
	for _, item := range firstSplit {
		isSingle := true
		if strings.HasPrefix(item, " ") {
			item = item[1:]
			isSingle = false
		}
		if strings.HasSuffix(item, " ") {
			item = item[0 : len(item)-1]
			isSingle = false
		}
		if len(item) == 0 {
			continue
		}
		if isSingle {
			array = append(array, item)
		} else {
			add := strings.Split(item, " ")
			array = append(array, add...)
		}
	}
	return array
}
