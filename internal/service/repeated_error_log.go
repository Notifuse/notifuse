package service

import (
	"sync"
	"time"

	"github.com/Notifuse/notifuse/pkg/logger"
)

// repeatedErrorWindow is how long an error logged once is logged again at Debug only.
const repeatedErrorWindow = 5 * time.Minute

// repeatedErrorLog logs a failure that repeats at every tick once per window: an error whose
// text is the one it logged less than repeatedErrorWindow ago goes to Debug instead of Error.
// Its zero value is ready to use.
type repeatedErrorLog struct {
	mu   sync.Mutex
	last string
	at   time.Time
	now  func() time.Time // the clock; time.Now when nil
}

// log writes message at Error the first time text is seen in the window, at Debug otherwise.
func (l *repeatedErrorLog) log(entry logger.Logger, text, message string) {
	if l.isNew(text) {
		entry.Error(message)
		return
	}
	entry.Debug(message)
}

func (l *repeatedErrorLog) isNew(text string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now
	if l.now != nil {
		now = l.now
	}
	t := now()
	if text == l.last && t.Sub(l.at) < repeatedErrorWindow {
		return false
	}
	l.last = text
	l.at = t
	return true
}
