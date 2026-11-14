package service

import (
	"testing"
	"time"

	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"

	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// Within the window every failure goes to Debug, whatever its text (the caller's entry carries
// the error); the first one after the window is logged at Error and opens a new window.
func TestRepeatedErrorLog(t *testing.T) {
	ctrl := gomock.NewController(t)
	log := pkgmocks.NewMockLogger(ctrl)

	clock := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	var l repeatedErrorLog
	l.now = func() time.Time { return clock }

	gomock.InOrder(
		log.EXPECT().Error("first"),
		log.EXPECT().Debug("again within the window"),
		log.EXPECT().Debug("still within the window"),
		log.EXPECT().Error("after the window"),
		log.EXPECT().Debug("within the new window"),
	)

	l.log(log, "first")
	clock = clock.Add(repeatedErrorWindow / 2)
	l.log(log, "again within the window")
	clock = clock.Add(repeatedErrorWindow/2 - time.Second)
	l.log(log, "still within the window")
	clock = clock.Add(time.Second)
	l.log(log, "after the window")
	clock = clock.Add(repeatedErrorWindow - time.Second)
	l.log(log, "within the new window")
}

func TestRepeatedErrorLog_ZeroValueUsesTheWallClock(t *testing.T) {
	ctrl := gomock.NewController(t)
	log := pkgmocks.NewMockLogger(ctrl)
	gomock.InOrder(
		log.EXPECT().Error("first"),
		log.EXPECT().Debug("second"),
	)

	var l repeatedErrorLog
	l.log(log, "first")
	l.log(log, "second")
}

var _ logger.Logger = (*pkgmocks.MockLogger)(nil)
