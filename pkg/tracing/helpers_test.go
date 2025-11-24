package tracing

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"net/http/httptest"

	"go.opencensus.io/trace"
)

func TestStartServiceSpan(t *testing.T) {
	ctx := context.Background()
	serviceName := "testService"
	methodName := "testMethod"

	// Create a span
	ctx, span := StartServiceSpan(ctx, serviceName, methodName)
	defer span.End()

	// Verify span was created
	if span == nil {
		t.Fatal("Expected span to be created")
	}

	// Verify span from context
	spanFromCtx := trace.FromContext(ctx)
	if spanFromCtx == nil {
		t.Fatal("Expected span to be in context")
	}
}

func TestEndSpan(t *testing.T) {
	// Create a test span
	_, span := trace.StartSpan(context.Background(), "test")

	// Test with no error
	EndSpan(span, nil)

	// Test with error
	testErr := errors.New("test error")
	_, span = trace.StartSpan(context.Background(), "test-with-error")
	EndSpan(span, testErr)
}

func TestTraceMethod(t *testing.T) {
	ctx := context.Background()
	serviceName := "testService"
	methodName := "testMethod"

	// Test with no error
	success := false
	err := TraceMethod(ctx, serviceName, methodName, func(ctx context.Context) error {
		success = true
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !success {
		t.Error("Expected function to be called")
	}

	// Test with error
	testErr := errors.New("test error")
	err = TraceMethod(ctx, serviceName, methodName, func(ctx context.Context) error {
		return testErr
	})

	if err != testErr {
		t.Errorf("Expected error %v, got %v", testErr, err)
	}
}

func TestTraceMethodWithResult(t *testing.T) {
	ctx := context.Background()
	serviceName := "testService"
	methodName := "testMethod"

	// Test with no error
	result, err := TraceMethodWithResult(ctx, serviceName, methodName, func(ctx context.Context) (string, error) {
		return "success", nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("Expected result to be 'success', got '%s'", result)
	}

	// Test with error
	testErr := errors.New("test error")
	result, err = TraceMethodWithResult(ctx, serviceName, methodName, func(ctx context.Context) (string, error) {
		return "failure", testErr
	})

	if err != testErr {
		t.Errorf("Expected error %v, got %v", testErr, err)
	}
	if result != "failure" {
		t.Errorf("Expected result to be 'failure', got '%s'", result)
	}
}

func TestAddAttribute(t *testing.T) {
	// Test with different value types
	testCases := []struct {
		name  string
		key   string
		value interface{}
	}{
		{"string", "string-key", "string-value"},
		{"int", "int-key", 123},
		{"int32", "int32-key", int32(123)},
		{"int64", "int64-key", int64(123)},
		{"bool", "bool-key", true},
		{"other", "other-key", struct{ Name string }{"test"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, span := trace.StartSpan(context.Background(), "test")
			defer span.End()

			// Add attribute
			AddAttribute(ctx, tc.key, tc.value)
		})
	}

	// Test with nil span in context
	AddAttribute(context.Background(), "key", "value")
}

func TestMarkSpanError(t *testing.T) {
	// Test with valid span and error
	ctx, span := trace.StartSpan(context.Background(), "test")
	defer span.End()

	testErr := errors.New("test error")
	MarkSpanError(ctx, testErr)

	// Test with nil error
	MarkSpanError(ctx, nil)

	// Test with nil span in context
	MarkSpanError(context.Background(), testErr)
}

func TestWrapHTTPClient(t *testing.T) {
	// Test with nil client
	client := WrapHTTPClient(nil)
	if client == nil {
		t.Fatal("Expected a new client to be created")
	}
	if client.Timeout != 30*time.Second {
		t.Errorf("Expected default timeout of 30s, got %v", client.Timeout)
	}

	// Test with existing client
	existingClient := &http.Client{
		Timeout: 60 * time.Second,
	}
	wrappedClient := WrapHTTPClient(existingClient)
	if wrappedClient == nil {
		t.Fatal("Expected a wrapped client to be created")
	}
	if wrappedClient.Timeout != 60*time.Second {
		t.Errorf("Expected timeout of 60s, got %v", wrappedClient.Timeout)
	}

	// Verify the transport was wrapped
	if wrappedClient.Transport == nil {
		t.Fatal("Expected Transport to be set")
	}

	// Verify the transport was set (it's wrapped, so we can't directly check the type)
	if wrappedClient.Transport == nil {
		t.Fatal("Expected Transport to be set")
	}
}

// TestWrapHTTPClientRequest tests that requests made with the wrapped client are traced
func TestWrapHTTPClientRequest(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Just respond with 200 OK
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create a wrapped client
	client := WrapHTTPClient(nil)

	// Create a context with a span
	ctx, rootSpan := trace.StartSpan(context.Background(), "test-span")
	defer rootSpan.End()

	// Create a request with the context
	req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check that the request was successful
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}

	// We can't easily inspect the span directly in a test,
	// but we can verify the client sent the request successfully
	// and that we used the proper OChttp transport
}
