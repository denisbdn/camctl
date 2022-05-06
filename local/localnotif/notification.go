package localnotif

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// NotificationData struct for describe notification object
type NotificationData struct {
	Method string
	Name   string
	Header http.Header
	Data   []byte
}

// Notification struct for describe notification server
type Notification struct {
	URL     string `json:"url,omitempty"`
	Key     string `json:"key,omitempty"`
	Value   string `json:"value,omitempty"`
	Channel chan *NotificationData
}

func buildClient() *http.Client {
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 100,
		},
		Timeout: 400 * time.Millisecond,
	}
	return client
}

// Notify it is long function witch read Channel n.Channel and send requests to n.url
func (n *Notification) Notify(log *zap.Logger) {
	log.Sugar().Warnf("start notify for url %s", n.URL)

	client := buildClient()
	url := n.URL
	if strings.HasSuffix(url, "/") {
		url = url[0 : len(url)-1]
	}

	for Data := range n.Channel {
		name := Data.Name
		if len(name) > 0 && !strings.HasPrefix(name, "/") {
			name = "/" + name
		}
		req, err := http.NewRequest(Data.Method, fmt.Sprintf("%s%s", url, name), bytes.NewBuffer(Data.Data))
		if Data.Header != nil {
			for k, a := range Data.Header {
				for _, v := range a {
					req.Header.Add(k, v)
				}
			}
		}
		if len(n.Key) > 0 {
			if len(n.Value) > 0 {
				req.Header.Add(n.Key, n.Value)
			} else {
				req.Header.Add(n.Key, "")
			}
		}
		if err != nil {
			log.Error("Notification error", zap.String("url", req.URL.String()), zap.Error(err))
		}
		res, err := client.Do(req)
		// 2-я попытка
		if err != nil {
			client = buildClient()
			res, err = client.Do(req)
		}
		if err == nil {
			_, errCopy := io.Copy(ioutil.Discard, res.Body)
			if errCopy == nil {
				res.Body.Close()
			}
			log.Warn("Notification", zap.String("url", req.URL.String()), zap.Int("responceCode", res.StatusCode))
		} else {
			log.Error("Repeat Notification error", zap.String("url", req.URL.String()), zap.Error(err))
			client = buildClient()
		}
	}

	log.Sugar().Warnf("stop notify for url %s", n.URL)
}

// Notifications struct for store several servers for notification
type Notifications []*Notification

// Send function send notification Data to array of notification servers
func (nf *Notifications) Send(log *zap.Logger, n *NotificationData) (sended int, skipped int) {
	sended = 0
	skipped = 0
	for _, notif := range *nf {
		select {
		case notif.Channel <- n:
			log.Warn("sent DELETE notification", zap.String("url", notif.URL+"/"+n.Name))
			sended++
		default:
			log.Error("not sent DELETE notification", zap.String("url", notif.URL+"/"+n.Name))
			skipped++
		}
	}
	return
}

// Close function close Channel of notification servers
func (nf *Notifications) Close() (closed int) {
	closed = 0
	for _, notif := range *nf {
		close(notif.Channel)
		notif.Channel = nil
		closed++
	}
	return
}

type Webhook struct {
	URL string `json:"url,omitempty"`
}

func (w *Webhook) Notify(log *zap.Logger) {
	log.Sugar().Warnf("start webhook for url %s", w.URL)
	_, err := http.Get(w.URL)
	if err != nil {
		log.Sugar().Errorf("error webhook for url %s", err.Error())
	}
	log.Sugar().Warnf("stop webhook for url %s", w.URL)
}

type Webhooks []*Webhook
