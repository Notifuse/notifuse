package mcp

import (
	"context"

	"github.com/Notifuse/notifuse/internal/domain"
)

// RequestMeta is what the /mcp HTTP handler knows about the request before
// the SDK takes it over: the values an audit row records.
type RequestMeta struct {
	IP        string
	UserAgent string
	RequestID string
}

type requestMetaKey struct{}

// WithRequestMeta returns a context carrying the request's metadata. The HTTP
// handler sets it next to the MCPCaller.
func WithRequestMeta(ctx context.Context, meta RequestMeta) context.Context {
	return context.WithValue(ctx, requestMetaKey{}, meta)
}

// RequestMetaFromContext returns the request's metadata, empty when the
// context did not come through the /mcp handler.
func RequestMetaFromContext(ctx context.Context) RequestMeta {
	meta, _ := ctx.Value(requestMetaKey{}).(RequestMeta)
	return meta
}

// callerFromContext returns the MCP caller the HTTP handler resolved, or nil.
func callerFromContext(ctx context.Context) *domain.MCPCaller {
	return domain.MCPCallerFromContext(ctx)
}
