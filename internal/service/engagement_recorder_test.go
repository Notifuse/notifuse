package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEngagementRecorder(t *testing.T) (*EngagementRecorder, *mocks.MockContactEngagementRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockContactEngagementRepository(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	return NewEngagementRecorder(repo, log), repo
}

func TestEngagementRecorder(t *testing.T) {
	ctx := context.Background()
	at := time.Now()

	t.Run("normalises the address and swallows an error", func(t *testing.T) {
		recorder, repo := newTestEngagementRecorder(t)
		repo.EXPECT().Touch(ctx, "ws", "bob@example.com", at).Return(errors.New("boom"))
		recorder.Touch(ctx, "ws", " Bob@Example.COM ", at)
	})

	t.Run("an empty address or message records nothing", func(t *testing.T) {
		recorder, _ := newTestEngagementRecorder(t)
		recorder.Touch(ctx, "ws", "", at)
		recorder.TouchForMessage(ctx, "ws", "", at)
		recorder.TouchForWebSessions(ctx, "ws", nil)
	})

	t.Run("a nil recorder records nothing", func(t *testing.T) {
		var recorder *EngagementRecorder
		recorder.Touch(ctx, "ws", "a@example.com", at)
		recorder.TouchForMessage(ctx, "ws", "m", at)
		recorder.TouchForWebSessions(ctx, "ws", []*domain.WebSession{{ID: "s"}})
	})

	t.Run("messages and sessions reach the repository; errors are swallowed", func(t *testing.T) {
		recorder, repo := newTestEngagementRecorder(t)
		sessions := []*domain.WebSession{{ID: "s"}}
		repo.EXPECT().TouchForMessage(ctx, "ws", "m", at).Return(errors.New("boom"))
		repo.EXPECT().TouchForWebSessions(ctx, "ws", sessions).Return(errors.New("boom"))
		recorder.TouchForMessage(ctx, "ws", "m", at)
		recorder.TouchForWebSessions(ctx, "ws", sessions)
	})
}

// The engagement clock counts only what the contact does (data retention plan, D5). Every
// place that records engagement is listed here: a new one is a conscious choice, made by
// adding it to this list.
func TestEngagementEntryPoints(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	call := regexp.MustCompile(`\.engagement\.Touch\w*\(`)
	found := map[string]int{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		if n := len(call.FindAll(src, -1)); n > 0 {
			found[f] = n
		}
	}
	var names []string
	for f := range found {
		names = append(names, f)
	}
	sort.Strings(names)
	assert.Equal(t, []string{
		"email_service.go",                 // VisitLink: a verified click on a real destination
		"inbound_webhook_event_service.go", // a reply that is not an auto-reply
		"list_service.go",                  // the contact's own subscription, left active
		"web_analytics_buffer.go",          // an identified visit
	}, names)
}
