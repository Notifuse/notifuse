package middleware

import (
	"context"
	"net/http"

	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/trace"
)

// TracingMiddleware adds OpenCensus tracing to HTTP requests
func TracingMiddleware(next http.Handler) http.Handler {
	handler := &ochttp.Handler{
		Handler: next,
		FormatSpanName: func(r *http.Request) string {
			return r.Method + " " + r.URL.Path
		},
		IsPublicEndpoint: true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add common attributes to all spans
		ctx := r.Context()
		spanCtx := trace.FromContext(ctx)
		if spanCtx != nil {
			// Add basic request information
			spanCtx.AddAttributes(
				trace.StringAttribute("http.host", r.Host),
				trace.StringAttribute("http.user_agent", r.UserAgent()),
				trace.StringAttribute("http.method", r.Method),
				trace.StringAttribute("http.path", r.URL.Path),
			)

			// Add query parameters as attributes (be careful with sensitive data)
			if r.URL.RawQuery != "" {
				spanCtx.AddAttributes(trace.StringAttribute("http.query", r.URL.RawQuery))
			}

			// Add request ID if available in headers
			if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
				spanCtx.AddAttributes(trace.StringAttribute("http.request_id", requestID))
			}

			// Add content type if available
			if contentType := r.Header.Get("Content-Type"); contentType != "" {
				spanCtx.AddAttributes(trace.StringAttribute("http.content_type", contentType))
			}
		}

		// Create a custom response writer to capture status code
		rw := &traceResponseWriter{
			ResponseWriter: w,
			ctx:            ctx,
		}

		// Process the request with OpenCensus tracing
		handler.ServeHTTP(rw, r)
	})
}

// traceResponseWriter is a custom response writer that captures status code
// for tracing purposes
type traceResponseWriter struct {
	http.ResponseWriter
	ctx        context.Context
	statusCode int
}

// WriteHeader captures the status code for tracing
func (trw *traceResponseWriter) WriteHeader(code int) {
	trw.statusCode = code

	// Add status code to span
	if span := trace.FromContext(trw.ctx); span != nil {
		span.AddAttributes(trace.Int64Attribute("http.status_code", int64(code)))

		// Mark error spans for 4xx and 5xx status codes
		if code >= 400 {
			span.SetStatus(trace.Status{
				Code:    trace.StatusCodeUnknown,
				Message: http.StatusText(code),
			})
		}
	}

	trw.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher for SSE streaming support
func (trw *traceResponseWriter) Flush() {
	if flusher, ok := trw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Ensure traceResponseWriter implements http.ResponseWriter and http.Flusher
var _ http.ResponseWriter = (*traceResponseWriter)(nil)
var _ http.Flusher = (*traceResponseWriter)(nil)
