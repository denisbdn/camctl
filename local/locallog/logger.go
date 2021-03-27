package locallog

import (
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// BuffLog struct for hold additional fields for logger
type BuffLog struct {
	Log         *zap.Logger
	array       []zapcore.Entry
	head        int
	isClosed    bool
	mut         sync.Mutex
	messages    chan zapcore.Entry
	subscribers []chan<- zapcore.Entry
}

func (l *BuffLog) messHandler(mess zapcore.Entry) {
	l.mut.Lock()
	defer l.mut.Unlock()
	l.array[l.head] = mess
	l.head++
	if l.head == len(l.array) {
		l.head = 0
	}
	for i := 0; i < len(l.subscribers); i++ {
		select {
		case l.subscribers[i] <- mess:
		default:
		}
	}
}

func handler(l *BuffLog) {
	for mess := range l.messages {
		l.messHandler(mess)
	}
}

// Close inner channel. and stop gorutine
func (l *BuffLog) Close() {
	l.mut.Lock()
	defer l.mut.Unlock()
	close(l.messages)
	l.messages = nil
	for _, curr := range l.subscribers {
		close(curr)
	}
	l.subscribers = l.subscribers[:0]
}

// NewBuffLog Logger over log.Logger this addition properties - listener and capacity
func NewBuffLog(logger *zap.Logger, capacity int) *BuffLog {
	res := new(BuffLog)
	res.array = make([]zapcore.Entry, capacity)
	res.head = 0
	res.isClosed = false
	res.mut = sync.Mutex{}
	res.messages = make(chan zapcore.Entry, capacity)
	res.subscribers = make([]chan<- zapcore.Entry, 0)
	go handler(res)
	res.Log = logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		res.mut.Lock()
		defer res.mut.Unlock()
		if res.messages == nil {
			return nil
		}
		select {
		case res.messages <- entry:
		default:
		}
		return nil
	}))
	return res
}

func revert(array []zapcore.Entry) []zapcore.Entry {
	for i := 0; i < len(array)/2; i++ {
		tmp := array[i]
		array[i] = array[len(array)-1-i]
		array[len(array)-1-i] = tmp
	}
	return array
}

// Buffer old log buffer whith capacity
func (l *BuffLog) Buffer(capacity int) []zapcore.Entry {
	l.mut.Lock()
	defer l.mut.Unlock()
	if capacity > len(l.array) {
		capacity = len(l.array)
	}
	res := make([]zapcore.Entry, 0, capacity)
	for i := l.head - 1; i >= 0; i-- {
		if len(res) >= capacity {
			return revert(res)
		}
		res = append(res, l.array[i])
	}
	for i := len(l.array) - 1; i >= l.head; i-- {
		if len(res) >= capacity {
			return revert(res)
		} else if len(l.array[i].Message) != 0 {
			res = append(res, l.array[i])
		} else {
			return revert(res)
		}
	}
	return revert(res)
}

// AddSubscriberBuffer register listener to new logs. Return old log buffer whith capacity
func (l *BuffLog) AddSubscriberBuffer(s chan<- zapcore.Entry, capacity int) []zapcore.Entry {
	l.mut.Lock()
	defer l.mut.Unlock()
	added := false
	for _, curr := range l.subscribers {
		if curr == s {
			added = true
			break
		}
	}
	if !added {
		l.subscribers = append(l.subscribers, s)
	}
	if capacity > len(l.array) {
		capacity = len(l.array)
	}
	res := make([]zapcore.Entry, 0, capacity)
	for i := l.head - 1; i >= 0; i-- {
		if len(res) >= capacity {
			return revert(res)
		}
		res = append(res, l.array[i])
	}
	for i := len(l.array) - 1; i >= l.head; i-- {
		if len(res) >= capacity {
			return revert(res)
		} else if len(l.array[i].Message) != 0 {
			res = append(res, l.array[i])
		} else {
			return revert(res)
		}
	}
	return revert(res)
}

// AddSubscriber register listener to new logs
func (l *BuffLog) AddSubscriber(s chan<- zapcore.Entry) int {
	l.mut.Lock()
	defer l.mut.Unlock()
	for _, curr := range l.subscribers {
		if curr == s {
			return 0
		}
	}
	l.subscribers = append(l.subscribers, s)
	return 1
}

// DelSubscriber unregister listener to new logs
func (l *BuffLog) DelSubscriber(s chan<- zapcore.Entry) int {
	l.mut.Lock()
	defer l.mut.Unlock()
	for i, curr := range l.subscribers {
		if curr == s {
			l.subscribers = append(l.subscribers[:i], l.subscribers[i+1:]...)
			return 1
		}
	}
	return 0
}
