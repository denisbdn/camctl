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
	OnStart       []*localnotif.Webhook      `json:"onstart,omitempty"`
	OnStop        []*localnotif.Webhook      `json:"onstop,omitempty"`
	OnError       []*localnotif.Webhook      `json:"onerror,omitempty"`
	Log           *locallog.BuffLog          `json:"-"`
}

// StreamFFMPEG describe cache object
type StreamFFMPEG struct {
	FFMPEG
	URLIn       string `json:"urlin,omitempty"`
	Port        uint   `json:"port,omitempty"`
	InitSegment string `json:"init,omitempty"`
	ExtraWindow uint   `json:"extra,omitempty"`
}

// StorageFFMPEG describe cache object
type StorageFFMPEG struct {
	FFMPEG
	URLIn         string `json:"urlin,omitempty"`
	URLOut        string `json:"urlout,omitempty"`
	ChankDuration uint   `json:"duration,omitempty"`
	StorageChanks uint   `json:"numbers,omitempty"`
}

func BuildFFMPEG(name string, workDir string, notifications []string, onstart []string, onstop []string, onerror []string) FFMPEG {
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
	onStart := make([]*localnotif.Webhook, 0)
	for _, str := range onstart {
		onStart = append(onStart, &localnotif.Webhook{URL: str})
	}
	onStop := make([]*localnotif.Webhook, 0)
	for _, str := range onstop {
		onStop = append(onStop, &localnotif.Webhook{URL: str})
	}
	onError := make([]*localnotif.Webhook, 0)
	for _, str := range onerror {
		onError = append(onError, &localnotif.Webhook{URL: str})
	}
	return FFMPEG{Name: name, Dir: workDir, TimeStr: nowStr, Notifications: nt, OnStart: onStart, OnStop: onStop, OnError: onError}
}

func BuildStreamFFMPEG(name string, workDir string, URLIn string, port uint, initSegment string, extraWindow uint, notifications []string, onstart []string, onstop []string, onerror []string) *StreamFFMPEG {
	data := StreamFFMPEG{URLIn: URLIn, Port: port, InitSegment: initSegment, ExtraWindow: extraWindow, FFMPEG: BuildFFMPEG(name, workDir, notifications, onstart, onstop, onerror)}
	return &data
}

func BuildStorageFFMPEG(name string, workDir string, URLIn string, URLOut string, ChankDuration uint, StorageChanks uint, notifications []string, onstart []string, onstop []string, onerror []string) *StorageFFMPEG {
	data := StorageFFMPEG{URLIn: URLIn, URLOut: URLOut, ChankDuration: ChankDuration, StorageChanks: StorageChanks, FFMPEG: BuildFFMPEG(name, workDir, notifications, onstart, onstop, onerror)}
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
