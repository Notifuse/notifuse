package service

import (
	"sync"
	"time"

	"github.com/Notifuse/notifuse/pkg/logger"
)

// repeatedErrorWindow is how long after an error logged at Error the next ones of the same
// call site go to Debug.
const repeatedErrorWindow = 5 * time.Minute

// repeatedErrorLog logs a failure that repeats at every tick at Error once per window, and at
// Debug in between. It is kept per call site and does not compare the error texts: a text that
// carries a count, or the list of the workspaces that failed, changes from one tick to the
// next. Its zero value is ready to use.
type repeatedErrorLog struct {
	mu     sync.Mutex
	logged bool
	at     time.Time
	now    func() time.Time // the clock; time.Now when nil
}

// log writes message at Error when no error was logged at Error in the window, at Debug
// otherwise.
func (l *repeatedErrorLog) log(entry logger.Logger, message string) {
	if l.due() {
		entry.Error(message)
		return
	}
	entry.Debug(message)
}

func (l *repeatedErrorLog) due() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now
	if l.now != nil {
		now = l.now
	}
	t := now()
	if l.logged && t.Sub(l.at) < repeatedErrorWindow {
		return false
	}
	l.logged = true
	l.at = t
	return true
}
