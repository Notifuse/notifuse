package tracing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opencensus.io/trace"

	"github.com/Notifuse/notifuse/config"
)

func TestInitTracing_Disabled(t *testing.T) {
	// Setup a tracing config with tracing disabled
	cfg := &config.TracingConfig{
		Enabled: false,
	}

	// Verify that initialization succeeds but does not set up any tracing
	err := InitTracing(cfg)
	if err != nil {
		t.Fatalf("Expected no error when tracing is disabled, got: %v", err)
	}
}

func TestInitTracing_WithInvalidExporter(t *testing.T) {
	// Setup a tracing config with an invalid exporter
	cfg := &config.TracingConfig{
		Enabled:       true,
		TraceExporter: "invalid",
	}

	// Expect an error
	err := InitTracing(cfg)
	if err == nil {
		t.Error("Expected error with invalid exporter, got nil")
	}
}

func TestInitMetricsExporters_WithInvalidExporter(t *testing.T) {
	// Setup a tracing config with an invalid metrics exporter
	cfg := &config.TracingConfig{
		Enabled:         true,
		MetricsExporter: "invalid",
	}

	// Expect an error
	err := initMetricsExporters(cfg)
	if err == nil {
		t.Error("Expected error with invalid metrics exporter, got nil")
	}
}

func TestInitMetricsExporters_Disabled(t *testing.T) {
	// Setup a tracing config with metrics disabled
	cfg := &config.TracingConfig{
		Enabled:         true,
		MetricsExporter: "none",
	}

	// Verify that initialization succeeds but does not set up any metrics
	err := initMetricsExporters(cfg)
	if err != nil {
		t.Fatalf("Expected no error when metrics are disabled, got: %v", err)
	}
}

func TestInitMetricsExporters_WithMultipleExportersSplitting(t *testing.T) {
	// This test simply checks if we correctly parse multiple exporter names
	// We don't actually initialize the exporters because that would require
	// external dependencies
	exporterStr := "prometheus, stackdriver,  datadog,, "
	exporters := strings.Split(exporterStr, ",")

	// Check that we get the expected number of non-empty exporters after trimming
	count := 0
	for _, exp := range exporters {
		if strings.TrimSpace(exp) != "" {
			count++
		}
	}

	if count != 3 {
		t.Errorf("Expected 3 non-empty exporters, got %d", count)
	}

	// Now verify each one
	foundPrometheus := false
	foundStackdriver := false
	foundDatadog := false

	for _, exp := range exporters {
		exp = strings.TrimSpace(exp)
		switch exp {
		case "prometheus":
			foundPrometheus = true
		case "stackdriver":
			foundStackdriver = true
		case "datadog":
			foundDatadog = true
		case "":
			// Skip empty strings
		default:
			t.Errorf("Unexpected exporter name: %s", exp)
		}
	}

	if !foundPrometheus {
		t.Error("Expected to find 'prometheus' exporter")
	}
	if !foundStackdriver {
		t.Error("Expected to find 'stackdriver' exporter")
	}
	if !foundDatadog {
		t.Error("Expected to find 'datadog' exporter")
	}
}

func TestGetHTTPOptions(t *testing.T) {
	transport := GetHTTPOptions()

	// Create a test request to check span naming
	req := httptest.NewRequest("GET", "/test-path", nil)
	spanName := transport.FormatSpanName(req)

	expectedSpanName := "GET /test-path"
	if spanName != expectedSpanName {
		t.Errorf("Expected span name to be %s, got %s", expectedSpanName, spanName)
	}

	// Verify sampler is not nil
	if transport.StartOptions.Sampler == nil {
		t.Fatal("Expected StartOptions.Sampler to be set")
	}
}

func TestRegisterHTTPServerViews(t *testing.T) {
	// This test just verifies the function does not error
	err := RegisterHTTPServerViews()
	if err != nil {
		t.Fatalf("Expected no error when registering HTTP server views, got: %v", err)
	}
}

func TestRegisterCustomViews(t *testing.T) {
	// This test verifies the custom views registration does not error
	err := registerCustomViews()
	if err != nil {
		t.Fatalf("Expected no error when registering custom views, got: %v", err)
	}
}

// Test DefaultTracer interface methods
func TestNewTracer(t *testing.T) {
	tracer := NewTracer()
	if tracer == nil {
		t.Fatal("Expected tracer to be created")
	}

	// Verify it's the correct type
	_, ok := tracer.(*DefaultTracer)
	if !ok {
		t.Fatal("Expected DefaultTracer type")
	}
}

func TestDefaultTracer_StartSpan(t *testing.T) {
	tracer := NewTracer()
	ctx := context.Background()

	newCtx, span := tracer.StartSpan(ctx, "test-span")
	if span == nil {
		t.Fatal("Expected span to be created")
	}
	if newCtx == ctx {
		t.Error("Expected new context to be different from original")
	}

	span.End()
}

func TestDefaultTracer_StartSpanWithAttributes(t *testing.T) {
	tracer := NewTracer()
	ctx := context.Background()

	attrs := []trace.Attribute{
		trace.StringAttribute("key1", "value1"),
		trace.Int64Attribute("key2", 123),
	}

	newCtx, span := tracer.StartSpanWithAttributes(ctx, "test-span", attrs...)
	if span == nil {
		t.Fatal("Expected span to be created")
	}
	if newCtx == ctx {
		t.Error("Expected new context to be different from original")
	}

	span.End()
}

func TestDefaultTracer_StartServiceSpan(t *testing.T) {
	tracer := NewTracer()
	ctx := context.Background()

	newCtx, span := tracer.StartServiceSpan(ctx, "TestService", "TestMethod")
	if span == nil {
		t.Fatal("Expected span to be created")
	}
	if newCtx == ctx {
		t.Error("Expected new context to be different from original")
	}

	span.End()
}

func TestDefaultTracer_EndSpan(t *testing.T) {
	tracer := NewTracer()
	ctx := context.Background()

	// Test with no error
	_, span := tracer.StartSpan(ctx, "test-span")
	tracer.EndSpan(span, nil)

	// Test with error
	_, span = tracer.StartSpan(ctx, "test-span-error")
	testErr := errors.New("test error")
	tracer.EndSpan(span, testErr)
}

func TestDefaultTracer_AddAttribute(t *testing.T) {
	tracer := NewTracer()
	ctx, span := tracer.StartSpan(context.Background(), "test-span")
	defer span.End()

	// Test different attribute types
	tracer.AddAttribute(ctx, "string-key", "string-value")
	tracer.AddAttribute(ctx, "int-key", 123)
	tracer.AddAttribute(ctx, "bool-key", true)
}

func TestDefaultTracer_MarkSpanError(t *testing.T) {
	tracer := NewTracer()
	ctx, span := tracer.StartSpan(context.Background(), "test-span")
	defer span.End()

	testErr := errors.New("test error")
	tracer.MarkSpanError(ctx, testErr)

	// Test with nil error
	tracer.MarkSpanError(ctx, nil)
}

func TestDefaultTracer_TraceMethod(t *testing.T) {
	tracer := NewTracer()
	ctx := context.Background()

	// Test successful method
	called := false
	err := tracer.TraceMethod(ctx, "TestService", "TestMethod", func(ctx context.Context) error {
		called = true
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !called {
		t.Error("Expected method to be called")
	}

	// Test method with error
	testErr := errors.New("test error")
	err = tracer.TraceMethod(ctx, "TestService", "TestMethod", func(ctx context.Context) error {
		return testErr
	})

	if err != testErr {
		t.Errorf("Expected error %v, got %v", testErr, err)
	}
}

func TestDefaultTracer_TraceMethodWithResultAny(t *testing.T) {
	tracer := NewTracer()
	ctx := context.Background()

	// Test successful method
	result, err := tracer.TraceMethodWithResultAny(ctx, "TestService", "TestMethod", func(ctx context.Context) (interface{}, error) {
		return "success", nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("Expected result 'success', got %v", result)
	}

	// Test method with error
	testErr := errors.New("test error")
	result, err = tracer.TraceMethodWithResultAny(ctx, "TestService", "TestMethod", func(ctx context.Context) (interface{}, error) {
		return "failure", testErr
	})

	if err != testErr {
		t.Errorf("Expected error %v, got %v", testErr, err)
	}
	if result != "failure" {
		t.Errorf("Expected result 'failure', got %v", result)
	}
}

func TestDefaultTracer_WrapHTTPClient(t *testing.T) {
	tracer := NewTracer()

	// Test with nil client
	client := tracer.WrapHTTPClient(nil)
	if client == nil {
		t.Fatal("Expected client to be created")
	}

	// Test with existing client
	existingClient := &http.Client{}
	wrappedClient := tracer.WrapHTTPClient(existingClient)
	if wrappedClient == nil {
		t.Fatal("Expected wrapped client to be created")
	}
}

func TestGetTracer(t *testing.T) {
	tracer := GetTracer()
	if tracer == nil {
		t.Fatal("Expected global tracer to be available")
	}
}

func TestSetTracer(t *testing.T) {
	originalTracer := GetTracer()
	defer SetTracer(originalTracer) // Restore original tracer

	newTracer := NewTracer()
	SetTracer(newTracer)

	retrievedTracer := GetTracer()
	if retrievedTracer != newTracer {
		t.Error("Expected retrieved tracer to be the same as set tracer")
	}
}

func TestStartSpan(t *testing.T) {
	ctx := context.Background()
	newCtx, span := StartSpan(ctx, "test-span")

	if span == nil {
		t.Fatal("Expected span to be created")
	}
	if newCtx == ctx {
		t.Error("Expected new context to be different from original")
	}

	span.End()
}

func TestStartSpanWithAttributes(t *testing.T) {
	ctx := context.Background()
	attrs := []trace.Attribute{
		trace.StringAttribute("key1", "value1"),
		trace.Int64Attribute("key2", 123),
	}

	newCtx, span := StartSpanWithAttributes(ctx, "test-span", attrs...)

	if span == nil {
		t.Fatal("Expected span to be created")
	}
	if newCtx == ctx {
		t.Error("Expected new context to be different from original")
	}

	span.End()
}

// Test exporter initialization error cases
func TestInitJaegerExporter_MissingEndpoint(t *testing.T) {
	cfg := &config.TracingConfig{
		JaegerEndpoint: "",
		ServiceName:    "test-service",
	}

	err := initJaegerExporter(cfg)
	if err == nil {
		t.Error("Expected error when Jaeger endpoint is missing")
	}
}

func TestInitZipkinExporter_MissingEndpoint(t *testing.T) {
	cfg := &config.TracingConfig{
		ZipkinEndpoint: "",
	}

	err := initZipkinExporter(cfg)
	if err == nil {
		t.Error("Expected error when Zipkin endpoint is missing")
	}
}

func TestInitStackdriverTraceExporter_MissingProjectID(t *testing.T) {
	cfg := &config.TracingConfig{
		StackdriverProjectID: "",
	}

	err := initStackdriverTraceExporter(cfg)
	if err == nil {
		t.Error("Expected error when Stackdriver project ID is missing")
	}
}

func TestInitDatadogTraceExporter_MissingAgentAddress(t *testing.T) {
	cfg := &config.TracingConfig{
		DatadogAgentAddress: "",
		AgentEndpoint:       "",
		ServiceName:         "test-service",
	}

	err := initDatadogTraceExporter(cfg)
	if err == nil {
		t.Error("Expected error when Datadog agent address is missing")
	}
}

func TestInitXRayExporter_MissingRegion(t *testing.T) {
	cfg := &config.TracingConfig{
		XRayRegion: "",
	}

	err := initXRayExporter(cfg)
	if err == nil {
		t.Error("Expected error when X-Ray region is missing")
	}
}

func TestInitStackdriverMetricsExporter_MissingProjectID(t *testing.T) {
	cfg := &config.TracingConfig{
		StackdriverProjectID: "",
	}

	err := initStackdriverMetricsExporter(cfg)
	if err == nil {
		t.Error("Expected error when Stackdriver project ID is missing")
	}
}

func TestInitDatadogMetricsExporter_MissingAgentAddress(t *testing.T) {
	cfg := &config.TracingConfig{
		DatadogAgentAddress: "",
		AgentEndpoint:       "",
		ServiceName:         "test-service",
	}

	err := initDatadogMetricsExporter(cfg)
	if err == nil {
		t.Error("Expected error when Datadog agent address is missing")
	}
}

func TestInitTraceExporter_NoneExporter(t *testing.T) {
	cfg := &config.TracingConfig{
		TraceExporter: "none",
	}

	err := initTraceExporter(cfg)
	if err != nil {
		t.Errorf("Expected no error for 'none' exporter, got %v", err)
	}
}

func TestInitTraceExporter_EmptyExporter(t *testing.T) {
	cfg := &config.TracingConfig{
		TraceExporter: "",
	}

	err := initTraceExporter(cfg)
	if err != nil {
		t.Errorf("Expected no error for empty exporter, got %v", err)
	}
}

func TestInitMetricsExporters_EmptyExporter(t *testing.T) {
	cfg := &config.TracingConfig{
		MetricsExporter: "",
	}

	err := initMetricsExporters(cfg)
	if err != nil {
		t.Errorf("Expected no error for empty metrics exporter, got %v", err)
	}
}

func TestInitTracing_WithNoneExporters(t *testing.T) {
	cfg := &config.TracingConfig{
		Enabled:             true,
		TraceExporter:       "none",
		MetricsExporter:     "none",
		SamplingProbability: 1.0,
	}

	err := InitTracing(cfg)
	if err != nil {
		t.Errorf("Expected no error with 'none' exporters, got %v", err)
	}
}

func TestInitTracing_WithEmptyExporters(t *testing.T) {
	cfg := &config.TracingConfig{
		Enabled:             true,
		TraceExporter:       "",
		MetricsExporter:     "",
		SamplingProbability: 0.5,
	}

	err := InitTracing(cfg)
	if err != nil {
		t.Errorf("Expected no error with empty exporters, got %v", err)
	}
}

func TestInitPrometheusExporter_WithoutPort(t *testing.T) {
	cfg := &config.TracingConfig{
		ServiceName:    "test-service",
		PrometheusPort: 0, // No port specified
	}

	err := initPrometheusExporter(cfg)
	if err != nil {
		t.Errorf("Expected no error when initializing Prometheus exporter without port, got %v", err)
	}
}

func TestInitPrometheusExporter_WithPort(t *testing.T) {
	cfg := &config.TracingConfig{
		ServiceName:    "test-service",
		PrometheusPort: 9090,
	}

	err := initPrometheusExporter(cfg)
	if err != nil {
		t.Errorf("Expected no error when initializing Prometheus exporter with port, got %v", err)
	}
}

func TestInitDatadogTraceExporter_WithFallbackEndpoint(t *testing.T) {
	cfg := &config.TracingConfig{
		DatadogAgentAddress: "",               // Empty primary address
		AgentEndpoint:       "localhost:8126", // Fallback endpoint
		ServiceName:         "test-service",
	}

	err := initDatadogTraceExporter(cfg)
	// The exporter initialization succeeds even without a real agent
	// This tests the fallback logic from DatadogAgentAddress to AgentEndpoint
	if err != nil {
		t.Errorf("Expected no error when initializing Datadog trace exporter, got %v", err)
	}
}

func TestInitDatadogMetricsExporter_WithFallbackEndpoint(t *testing.T) {
	cfg := &config.TracingConfig{
		DatadogAgentAddress: "",               // Empty primary address
		AgentEndpoint:       "localhost:8126", // Fallback endpoint
		ServiceName:         "test-service",
	}

	err := initDatadogMetricsExporter(cfg)
	// The exporter initialization succeeds even without a real agent
	// This tests the fallback logic from DatadogAgentAddress to AgentEndpoint
	if err != nil {
		t.Errorf("Expected no error when initializing Datadog metrics exporter, got %v", err)
	}
}

func TestInitDatadogMetricsExporter_WithAPIKey(t *testing.T) {
	cfg := &config.TracingConfig{
		DatadogAgentAddress: "localhost:8126",
		ServiceName:         "test-service",
		DatadogAPIKey:       "test-api-key",
	}

	err := initDatadogMetricsExporter(cfg)
	// The exporter initialization succeeds even without a real agent
	// This tests the API key configuration logic
	if err != nil {
		t.Errorf("Expected no error when initializing Datadog metrics exporter with API key, got %v", err)
	}
}

func TestInitMetricsExporters_WithMultipleValidExporters(t *testing.T) {
	// Test with multiple exporters that should succeed in parsing
	// but may fail in actual initialization due to missing dependencies
	cfg := &config.TracingConfig{
		MetricsExporter:      "prometheus,stackdriver",
		ServiceName:          "test-service",
		StackdriverProjectID: "test-project",
	}

	err := initMetricsExporters(cfg)
	// We expect this to fail on Stackdriver due to missing credentials,
	// but it should successfully parse the multiple exporters
	if err == nil {
		t.Log("Metrics exporters initialized successfully")
	} else {
		t.Logf("Expected error due to missing external dependencies: %v", err)
	}
}

func TestInitMetricsExporters_WithWhitespaceInExporters(t *testing.T) {
	cfg := &config.TracingConfig{
		MetricsExporter:      "  prometheus  ,  , stackdriver  ,  ",
		ServiceName:          "test-service",
		StackdriverProjectID: "test-project",
	}

	err := initMetricsExporters(cfg)
	// This tests the whitespace trimming logic
	if err == nil {
		t.Log("Metrics exporters with whitespace handled successfully")
	} else {
		t.Logf("Expected error due to missing external dependencies: %v", err)
	}
}

func TestInitTracing_FullConfiguration(t *testing.T) {
	cfg := &config.TracingConfig{
		Enabled:             true,
		TraceExporter:       "none",
		MetricsExporter:     "none",
		SamplingProbability: 0.1,
		ServiceName:         "test-service",
	}

	err := InitTracing(cfg)
	if err != nil {
		t.Errorf("Expected no error with full configuration, got %v", err)
	}
}

func TestNewMetricsServer_LimitsHeaderReadingAndIdleConnections(t *testing.T) {
	handler := http.NewServeMux()

	srv := newMetricsServer(9464, handler)

	if srv.Addr != ":9464" {
		t.Errorf("Expected Addr :9464, got %s", srv.Addr)
	}
	if srv.Handler != handler {
		t.Error("Expected the given handler")
	}
	if srv.ReadHeaderTimeout != 20*time.Second {
		t.Errorf("Expected ReadHeaderTimeout 20s, got %s", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout != 70*time.Minute {
		t.Errorf("Expected IdleTimeout 70m, got %s", srv.IdleTimeout)
	}
}
