package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestRequestMeta_RoundTrip(t *testing.T) {
	assert.Equal(t, RequestMeta{}, RequestMetaFromContext(context.Background()), "empty outside /mcp")

	meta := RequestMeta{IP: "203.0.113.7", UserAgent: "agent/1", RequestID: "req-1"}
	assert.Equal(t, meta, RequestMetaFromContext(WithRequestMeta(context.Background(), meta)))
}

func TestCallerFromContext(t *testing.T) {
	assert.Nil(t, callerFromContext(context.Background()))
	caller := &domain.MCPCaller{Kind: domain.MCPCredentialHeader, APIKeyUserID: "key-1"}
	assert.Same(t, caller, callerFromContext(domain.WithMCPCaller(context.Background(), caller)))
}
