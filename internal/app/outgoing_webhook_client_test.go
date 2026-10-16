package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/pkg/safehttpclient"
)

// Subscription deliveries and automation webhook nodes both take their client
// from newOutgoingWebhookClient, so the opt-out and the order of the tracing wrap
// are pinned here once for both paths. A tracing wrap applied before the guard
// was chosen, or one that dropped the base transport, would silently unguard
// every outgoing webhook on deployments that enable tracing.
func TestNewOutgoingWebhookClient(t *testing.T) {
	var hits int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	post := func(t *testing.T, client *http.Client) error {
		t.Helper()
		resp, err := client.Post(receiver.URL, "application/json", strings.NewReader(`{}`))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		return nil
	}

	for _, tc := range []struct {
		name              string
		allowPrivateHosts bool
		tracingEnabled    bool
	}{
		{name: "refuses a private target by default"},
		{name: "the guard survives tracing", tracingEnabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := atomic.LoadInt32(&hits)
			err := post(t, newOutgoingWebhookClient(tc.allowPrivateHosts, tc.tracingEnabled, 10*time.Second))
			require.Error(t, err)
			assert.ErrorIs(t, err, safehttpclient.ErrPrivateIP)
			assert.Equal(t, before, atomic.LoadInt32(&hits), "a refused target must never be contacted")
		})
	}

	for _, tc := range []struct {
		name           string
		tracingEnabled bool
	}{
		{name: "the opt-out reaches the private network"},
		{name: "the opt-out reaches the private network with tracing", tracingEnabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := atomic.LoadInt32(&hits)
			require.NoError(t, post(t, newOutgoingWebhookClient(true, tc.tracingEnabled, 10*time.Second)))
			assert.Equal(t, before+1, atomic.LoadInt32(&hits))
		})
	}

	// The delivery worker derives its claim lease from its client's timeout, and
	// the webhook node kept 30s from before it had a guard; neither may change
	// with the opt-out or with tracing.
	t.Run("keeps the timeout it is given", func(t *testing.T) {
		for _, allowPrivateHosts := range []bool{false, true} {
			for _, tracingEnabled := range []bool{false, true} {
				for _, timeout := range []time.Duration{10 * time.Second, 30 * time.Second} {
					client := newOutgoingWebhookClient(allowPrivateHosts, tracingEnabled, timeout)
					assert.Equal(t, timeout, client.Timeout,
						"allowPrivateHosts=%v tracingEnabled=%v", allowPrivateHosts, tracingEnabled)
				}
			}
		}
	})
}
