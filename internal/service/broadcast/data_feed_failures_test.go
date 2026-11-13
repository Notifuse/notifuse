package broadcast

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fieldRecorder is a mock logger that keeps every field set it is given.
type fieldRecorder struct {
	mu     sync.Mutex
	fields []map[string]interface{}
}

func newFieldRecorder(ctrl *gomock.Controller) (*pkgmocks.MockLogger, *fieldRecorder) {
	rec := &fieldRecorder{}
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithFields(gomock.Any()).DoAndReturn(func(fields map[string]interface{}) *pkgmocks.MockLogger {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.fields = append(rec.fields, fields)
		return log
	}).AnyTimes()
	log.EXPECT().Debug(gomock.Any()).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	return log, rec
}

// loggedBodies returns the "body" fields logged so far.
func (r *fieldRecorder) loggedBodies() []interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	var bodies []interface{}
	for _, f := range r.fields {
		if body, ok := f["body"]; ok {
			bodies = append(bodies, body)
		}
	}
	return bodies
}

// blockingServer holds every request until the client gives up, and signals
// each arrival on started.
func blockingServer(t *testing.T) (*httptest.Server, <-chan struct{}, *atomic.Int32) {
	t.Helper()
	started := make(chan struct{}, 8)
	release := make(chan struct{})
	hits := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		// The server only notices a client hang-up once the body is read.
		_, _ = io.Copy(io.Discard, r.Body)
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(server.Close)
	// Cleanups run last-in first-out: the handler is released before Close waits on it.
	t.Cleanup(func() { close(release) })
	return server, started, hits
}

// countingServer answers every request with status and body.
func countingServer(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	hits := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, hits
}

func recipientPayload() *domain.RecipientFeedRequestPayload {
	return &domain.RecipientFeedRequestPayload{
		Contact:   domain.RecipientFeedContact{Email: "test@example.com"},
		Broadcast: domain.RecipientFeedBroadcast{ID: "broadcast-123", Name: "Test Broadcast"},
		List:      domain.RecipientFeedList{ID: "list-456", Name: "Test List"},
		Workspace: domain.RecipientFeedWorkspace{ID: "ws-789", Name: "Test Workspace"},
	}
}

// A broadcast stopped while a recipient feed call is in flight gives up at once:
// the cancellation is not retried.
func TestDataFeedFetcher_FetchRecipient_CancelledInFlight_NotRetried(t *testing.T) {
	ctrl := gomock.NewController(t)
	log, _ := newFieldRecorder(ctrl)
	server, started, hits := blockingServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()

	result, err := NewUnsafeDataFeedFetcher(log).FetchRecipient(ctx,
		&domain.RecipientFeedSettings{Enabled: true, URL: server.URL}, recipientPayload())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "request cancelled")
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
	assert.Equal(t, int32(1), hits.Load(), "a cancelled request is not retried")
}

// A 200 that is not JSON is the feed's bug, not a transient fault: no retry.
func TestDataFeedFetcher_FetchRecipient_InvalidJSON_NotRetried(t *testing.T) {
	ctrl := gomock.NewController(t)
	log, _ := newFieldRecorder(ctrl)
	server, hits := countingServer(t, http.StatusOK, "<html>not json</html>")

	result, err := NewUnsafeDataFeedFetcher(log).FetchRecipient(context.Background(),
		&domain.RecipientFeedSettings{Enabled: true, URL: server.URL}, recipientPayload())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON response")
	assert.Nil(t, result)
	assert.Equal(t, int32(1), hits.Load())
}

// The error names the status; the log keeps the body, "(empty)" when there is none.
func TestDataFeedFetcher_FetchRecipient_HTTPErrorWithEmptyBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	log, rec := newFieldRecorder(ctrl)
	server, hits := countingServer(t, http.StatusBadRequest, "")

	result, err := NewUnsafeDataFeedFetcher(log).FetchRecipient(context.Background(),
		&domain.RecipientFeedSettings{Enabled: true, URL: server.URL}, recipientPayload())

	require.Error(t, err)
	assert.Equal(t, "HTTP error 400: Bad Request", err.Error())
	assert.Nil(t, result)
	assert.Equal(t, int32(1), hits.Load(), "a 4xx is not retried")
	assert.Equal(t, []interface{}{"(empty)"}, rec.loggedBodies())
}

// A nil payload is sent as an empty JSON object, and a JSON null answer is an
// empty result rather than nil.
func TestDataFeedFetcher_FetchRecipient_NilPayloadAndNullAnswer(t *testing.T) {
	ctrl := gomock.NewController(t)
	log, _ := newFieldRecorder(ctrl)
	var received atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		received.Store(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("null"))
	}))
	t.Cleanup(server.Close)

	result, err := NewUnsafeDataFeedFetcher(log).FetchRecipient(context.Background(),
		&domain.RecipientFeedSettings{Enabled: true, URL: server.URL}, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, true, result["_success"])
	assert.Equal(t, map[string]interface{}{}, received.Load())
}

func TestDataFeedFetcher_FetchGlobal_CancelledInFlight(t *testing.T) {
	ctrl := gomock.NewController(t)
	log, _ := newFieldRecorder(ctrl)
	server, started, hits := blockingServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()

	result, err := NewUnsafeDataFeedFetcher(log).FetchGlobal(ctx,
		&domain.GlobalFeedSettings{Enabled: true, URL: server.URL}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "request cancelled")
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
	assert.Equal(t, int32(1), hits.Load())
}

func TestDataFeedFetcher_FetchGlobal_HTTPErrorWithEmptyBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	log, rec := newFieldRecorder(ctrl)
	server, _ := countingServer(t, http.StatusServiceUnavailable, "")

	result, err := NewUnsafeDataFeedFetcher(log).FetchGlobal(context.Background(),
		&domain.GlobalFeedSettings{Enabled: true, URL: server.URL}, nil)

	require.Error(t, err)
	assert.Equal(t, "HTTP error 503: Service Unavailable", err.Error())
	assert.Nil(t, result)
	assert.Equal(t, []interface{}{"(empty)"}, rec.loggedBodies())
}

func TestDataFeedFetcher_FetchGlobal_NullAnswer(t *testing.T) {
	ctrl := gomock.NewController(t)
	log, _ := newFieldRecorder(ctrl)
	server, _ := countingServer(t, http.StatusOK, "null")

	result, err := NewUnsafeDataFeedFetcher(log).FetchGlobal(context.Background(),
		&domain.GlobalFeedSettings{Enabled: true, URL: server.URL}, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, true, result["_success"])
}

// Statuses without a curated description fall back to the standard text, then
// to a generic one.
func TestGetHTTPStatusDescription_Fallbacks(t *testing.T) {
	assert.Equal(t, "Not Found", getHTTPStatusDescription(http.StatusNotFound))
	assert.Equal(t, http.StatusText(http.StatusTeapot), getHTTPStatusDescription(http.StatusTeapot))
	assert.Equal(t, "Unknown Error", getHTTPStatusDescription(599))
}
