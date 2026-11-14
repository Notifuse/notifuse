package service

import (
	"testing"
	"time"

	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/golang/mock/gomock"

	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func TestRepeatedErrorLog(t *testing.T) {
	ctrl := gomock.NewController(t)
	log := pkgmocks.NewMockLogger(ctrl)

	clock := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	var l repeatedErrorLog
	l.now = func() time.Time { return clock }

	gomock.InOrder(
		log.EXPECT().Error("first"),
		log.EXPECT().Debug("again within the window"),
		log.EXPECT().Error("another text"),
		log.EXPECT().Error("the first text again"),
		log.EXPECT().Debug("still within the new window"),
		log.EXPECT().Error("after the window"),
	)

	l.log(log, "boom", "first")
	clock = clock.Add(repeatedErrorWindow - time.Second)
	l.log(log, "boom", "again within the window")
	l.log(log, "other", "another text")
	l.log(log, "boom", "the first text again")
	clock = clock.Add(repeatedErrorWindow - time.Second)
	l.log(log, "boom", "still within the new window")
	clock = clock.Add(time.Second)
	l.log(log, "boom", "after the window")
}

func TestRepeatedErrorLog_ZeroValueUsesTheWallClock(t *testing.T) {
	ctrl := gomock.NewController(t)
	log := pkgmocks.NewMockLogger(ctrl)
	gomock.InOrder(
		log.EXPECT().Error("first"),
		log.EXPECT().Debug("second"),
	)

	var l repeatedErrorLog
	l.log(log, "boom", "first")
	l.log(log, "boom", "second")
}

var _ logger.Logger = (*pkgmocks.MockLogger)(nil)
