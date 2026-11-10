package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opencensus.io/trace"
)

type recordingExporter struct {
	mu    sync.Mutex
	spans []*trace.SpanData
}

func (e *recordingExporter) ExportSpan(s *trace.SpanData) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, s)
}

// exported renders every exported span, name and attributes, as one string.
func (e *recordingExporter) exported() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var b strings.Builder
	for _, s := range e.spans {
		fmt.Fprintf(&b, "%s %v\n", s.Name, s.Attributes)
	}
	return b.String()
}

func TestTracingMiddleware_LeavesWebVersionTokensOutOfTraces(t *testing.T) {
	// A /v/ token opens a contact's email for anyone who holds it, with no expiry: it
	// must not reach the trace backend.
	exporter := &recordingExporter{}
	trace.RegisterExporter(exporter)
	t.Cleanup(func() { trace.UnregisterExporter(exporter) })
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
	t.Cleanup(func() { trace.ApplyConfig(trace.Config{DefaultSampler: trace.ProbabilitySampler(1e-4)}) })

	served := false
	handler := TracingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served = true
		w.WriteHeader(http.StatusOK)
	}))

	for _, withParent := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodGet, "/v/secret-view-token?utm_source=news", nil)
		var parent *trace.Span
		if withParent {
			var ctx = req.Context()
			ctx, parent = trace.StartSpan(ctx, "parent")
			req = req.WithContext(ctx)
		}
		handler.ServeHTTP(httptest.NewRecorder(), req)
		if parent != nil {
			parent.End()
		}
	}
	require.True(t, served)

	other := httptest.NewRequest(http.MethodGet, "/api/contacts.list", nil)
	handler.ServeHTTP(httptest.NewRecorder(), other)

	exported := exporter.exported()
	assert.Contains(t, exported, "/api/contacts.list", "other routes are still traced")
	assert.NotContains(t, exported, "secret-view-token")
}
