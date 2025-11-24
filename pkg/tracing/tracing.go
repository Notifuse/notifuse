package tracing

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"contrib.go.opencensus.io/exporter/aws"
	"contrib.go.opencensus.io/exporter/jaeger"
	"contrib.go.opencensus.io/exporter/prometheus"
	"contrib.go.opencensus.io/exporter/stackdriver"
	"contrib.go.opencensus.io/exporter/zipkin"
	"contrib.go.opencensus.io/integrations/ocsql"
	datadog "github.com/DataDog/opencensus-go-exporter-datadog"
	zipkinhttp "github.com/openzipkin/zipkin-go/reporter/http"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"

	"github.com/Notifuse/notifuse/config"
)

//go:generate mockgen -destination=../mocks/mock_tracer.go -package=pkgmocks github.com/Notifuse/notifuse/pkg/tracing Tracer

// Tracer defines the interface for tracing functionality
// codecov:ignore:start
type Tracer interface {
	// StartSpan starts a new span
	StartSpan(ctx context.Context, name string) (context.Context, *trace.Span)

	// StartSpanWithAttributes starts a new span with attributes
	StartSpanWithAttributes(ctx context.Context, name string, attrs ...trace.Attribute) (context.Context, *trace.Span)

	// StartServiceSpan starts a new span for a service method
	StartServiceSpan(ctx context.Context, serviceName, methodName string) (context.Context, *trace.Span)

	// EndSpan ends a span and records any error
	EndSpan(span *trace.Span, err error)

	// AddAttribute adds an attribute to the current span
	AddAttribute(ctx context.Context, key string, value interface{})

	// MarkSpanError marks the current span as failed with the given error
	MarkSpanError(ctx context.Context, err error)

	// TraceMethod is a helper to trace a service method with automatic span ending
	TraceMethod(ctx context.Context, serviceName, methodName string, f func(context.Context) error) error

	// TraceMethodWithResult is a helper to trace a service method that returns a result
	// Note: Due to Go interface limitations, we use interface{} instead of generics
	TraceMethodWithResultAny(ctx context.Context, serviceName, methodName string, f func(context.Context) (interface{}, error)) (interface{}, error)

	// WrapHTTPClient wraps an http.Client with OpenCensus tracing
	WrapHTTPClient(client *http.Client) *http.Client
}

// DefaultTracer is the default implementation of the Tracer interface
type DefaultTracer struct{}

// NewTracer creates a new DefaultTracer
func NewTracer() Tracer {
	return &DefaultTracer{}
}

// StartSpan implements Tracer.StartSpan
func (t *DefaultTracer) StartSpan(ctx context.Context, name string) (context.Context, *trace.Span) {
	return StartSpan(ctx, name)
}

// StartSpanWithAttributes implements Tracer.StartSpanWithAttributes
func (t *DefaultTracer) StartSpanWithAttributes(ctx context.Context, name string, attrs ...trace.Attribute) (context.Context, *trace.Span) {
	return StartSpanWithAttributes(ctx, name, attrs...)
}

// StartServiceSpan implements Tracer.StartServiceSpan
func (t *DefaultTracer) StartServiceSpan(ctx context.Context, serviceName, methodName string) (context.Context, *trace.Span) {
	return StartServiceSpan(ctx, serviceName, methodName)
}

// EndSpan implements Tracer.EndSpan
func (t *DefaultTracer) EndSpan(span *trace.Span, err error) {
	EndSpan(span, err)
}

// AddAttribute implements Tracer.AddAttribute
func (t *DefaultTracer) AddAttribute(ctx context.Context, key string, value interface{}) {
	AddAttribute(ctx, key, value)
}

// MarkSpanError implements Tracer.MarkSpanError
func (t *DefaultTracer) MarkSpanError(ctx context.Context, err error) {
	MarkSpanError(ctx, err)
}

// TraceMethod implements Tracer.TraceMethod
func (t *DefaultTracer) TraceMethod(ctx context.Context, serviceName, methodName string, f func(context.Context) error) error {
	return TraceMethod(ctx, serviceName, methodName, f)
}

// TraceMethodWithResultAny implements Tracer.TraceMethodWithResultAny
func (t *DefaultTracer) TraceMethodWithResultAny(ctx context.Context, serviceName, methodName string, f func(context.Context) (interface{}, error)) (interface{}, error) {
	ctx, span := StartServiceSpan(ctx, serviceName, methodName)
	defer span.End()

	result, err := f(ctx)
	if err != nil {
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeUnknown,
			Message: err.Error(),
		})
	}

	return result, err
}

// WrapHTTPClient implements Tracer.WrapHTTPClient
func (t *DefaultTracer) WrapHTTPClient(client *http.Client) *http.Client {
	return WrapHTTPClient(client)
}

// Global instance of the tracer
var globalTracer Tracer = NewTracer()

// GetTracer returns the global tracer instance
func GetTracer() Tracer {
	return globalTracer
}

// SetTracer sets the global tracer instance
func SetTracer(tracer Tracer) {
	globalTracer = tracer
}

// codecov:ignore:end

// InitTracing initializes OpenCensus tracing with the given configuration
// codecov:ignore:start
func InitTracing(tracingConfig *config.TracingConfig) error {
	if !tracingConfig.Enabled {
		return nil
	}

	// Configure trace sampling rate
	trace.ApplyConfig(trace.Config{
		DefaultSampler: trace.ProbabilitySampler(tracingConfig.SamplingProbability),
	})

	// Initialize trace exporter based on configuration
	if tracingConfig.TraceExporter != "none" && tracingConfig.TraceExporter != "" {
		if err := initTraceExporter(tracingConfig); err != nil {
			return err
		}
	}

	// Initialize metrics exporters based on configuration
	if tracingConfig.MetricsExporter != "none" && tracingConfig.MetricsExporter != "" {
		if err := initMetricsExporters(tracingConfig); err != nil {
			return err
		}
	}

	// Register default views for HTTP metrics
	if err := view.Register(ochttp.DefaultServerViews...); err != nil {
		return fmt.Errorf("failed to register HTTP server views: %w", err)
	}

	log.Printf("OpenCensus initialized with trace exporter: %s, metrics exporters: %s",
		tracingConfig.TraceExporter, tracingConfig.MetricsExporter)
	return nil
}

// initTraceExporter initializes the trace exporter based on configuration
func initTraceExporter(cfg *config.TracingConfig) error {
	switch cfg.TraceExporter {
	case "jaeger":
		return initJaegerExporter(cfg)
	case "zipkin":
		return initZipkinExporter(cfg)
	case "stackdriver":
		return initStackdriverTraceExporter(cfg)
	case "datadog":
		return initDatadogTraceExporter(cfg)
	case "xray":
		return initXRayExporter(cfg)
	case "none", "":
		log.Printf("No trace exporter configured")
		return nil
	default:
		return fmt.Errorf("unsupported trace exporter: %s", cfg.TraceExporter)
	}
}

// initMetricsExporters initializes metrics exporters based on configuration
func initMetricsExporters(cfg *config.TracingConfig) error {
	// If no exporter is configured, return early
	if cfg.MetricsExporter == "none" || cfg.MetricsExporter == "" {
		log.Printf("No metrics exporter configured")
		return nil
	}

	// Split by comma to support multiple exporters
	exporters := strings.Split(cfg.MetricsExporter, ",")
	initializedExporters := make([]string, 0, len(exporters))

	for _, exporter := range exporters {
		exporter = strings.TrimSpace(exporter)
		if exporter == "" {
			continue
		}

		var err error
		switch exporter {
		case "prometheus":
			err = initPrometheusExporter(cfg)
		case "stackdriver":
			err = initStackdriverMetricsExporter(cfg)
		case "datadog":
			err = initDatadogMetricsExporter(cfg)
		default:
			return fmt.Errorf("unsupported metrics exporter: %s", exporter)
		}

		if err != nil {
			return fmt.Errorf("failed to initialize %s metrics exporter: %w", exporter, err)
		}

		initializedExporters = append(initializedExporters, exporter)
		log.Printf("Initialized %s metrics exporter", exporter)
	}

	// Register custom views for metrics
	if err := registerCustomViews(); err != nil {
		return fmt.Errorf("failed to register custom views: %w", err)
	}

	if len(initializedExporters) > 0 {
		log.Printf("Successfully initialized metrics exporters: %s", strings.Join(initializedExporters, ", "))
	} else {
		log.Printf("No valid metrics exporters found in configuration: %s", cfg.MetricsExporter)
	}

	return nil
}

// registerCustomViews registers custom metrics views
func registerCustomViews() error {
	// Register database views (from ocsql)
	if err := view.Register(ocsql.DefaultViews...); err != nil {
		return fmt.Errorf("failed to register database views: %w", err)
	}

	// Register additional custom views if needed
	// For example:
	// serviceLatencyView := &view.View{...}
	// if err := view.Register(serviceLatencyView); err != nil {
	//     return fmt.Errorf("failed to register service latency view: %w", err)
	// }

	return nil
}

// initJaegerExporter initializes the Jaeger exporter
func initJaegerExporter(cfg *config.TracingConfig) error {
	if cfg.JaegerEndpoint == "" {
		return fmt.Errorf("jaeger endpoint is required for Jaeger exporter")
	}

	je, err := jaeger.NewExporter(jaeger.Options{
		CollectorEndpoint: cfg.JaegerEndpoint,
		ServiceName:       cfg.ServiceName,
		Process: jaeger.Process{
			ServiceName: cfg.ServiceName,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create Jaeger exporter: %w", err)
	}

	trace.RegisterExporter(je)
	log.Printf("Jaeger exporter initialized with endpoint %s", cfg.JaegerEndpoint)
	return nil
}

// initZipkinExporter initializes the Zipkin exporter
func initZipkinExporter(cfg *config.TracingConfig) error {
	if cfg.ZipkinEndpoint == "" {
		return fmt.Errorf("zipkin endpoint is required for Zipkin exporter")
	}

	reporter := zipkinhttp.NewReporter(cfg.ZipkinEndpoint)
	ze := zipkin.NewExporter(reporter, nil)
	trace.RegisterExporter(ze)
	log.Printf("Zipkin exporter initialized with endpoint %s", cfg.ZipkinEndpoint)
	return nil
}

// initStackdriverTraceExporter initializes the Stackdriver trace exporter
func initStackdriverTraceExporter(cfg *config.TracingConfig) error {
	if cfg.StackdriverProjectID == "" {
		return fmt.Errorf("stackdriver project ID is required for Stackdriver exporter")
	}

	se, err := stackdriver.NewExporter(stackdriver.Options{
		ProjectID: cfg.StackdriverProjectID,
	})
	if err != nil {
		return fmt.Errorf("failed to create Stackdriver exporter: %w", err)
	}

	trace.RegisterExporter(se)
	log.Printf("Stackdriver exporter initialized with project ID %s", cfg.StackdriverProjectID)
	return nil
}

// initDatadogTraceExporter initializes the Datadog trace exporter
func initDatadogTraceExporter(cfg *config.TracingConfig) error {
	agentAddr := cfg.DatadogAgentAddress
	if agentAddr == "" {
		agentAddr = cfg.AgentEndpoint // Fall back to general agent endpoint
	}

	if agentAddr == "" {
		return fmt.Errorf("datadog agent address is required for Datadog exporter")
	}

	// Create Datadog exporter
	exporter, err := datadog.NewExporter(
		datadog.Options{
			Service:   cfg.ServiceName,
			TraceAddr: agentAddr,
			StatsAddr: agentAddr,
			Tags:      []string{"env:prod"},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create Datadog exporter: %w", err)
	}

	trace.RegisterExporter(exporter)
	log.Printf("Datadog exporter initialized with agent address %s", agentAddr)
	return nil
}

// initXRayExporter initializes the AWS X-Ray exporter
func initXRayExporter(cfg *config.TracingConfig) error {
	if cfg.XRayRegion == "" {
		return fmt.Errorf("AWS region is required for X-Ray exporter")
	}

	// Create AWS X-Ray exporter
	exporter, err := aws.NewExporter(
		aws.WithRegion(cfg.XRayRegion),
		aws.WithVersion("latest"),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS X-Ray exporter: %w", err)
	}

	trace.RegisterExporter(exporter)
	log.Printf("AWS X-Ray exporter initialized with region %s", cfg.XRayRegion)
	return nil
}

// initPrometheusExporter initializes the Prometheus exporter
func initPrometheusExporter(cfg *config.TracingConfig) error {
	// Create the Prometheus exporter with service name as namespace
	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: cfg.ServiceName,
		OnError: func(err error) {
			log.Printf("Prometheus exporter error: %v", err)
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create Prometheus exporter: %w", err)
	}

	// Register the exporter with OpenCensus for metrics
	view.RegisterExporter(pe)

	// Start a Prometheus HTTP server if port is specified
	if cfg.PrometheusPort > 0 {
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", pe)

			// Add a simple health check endpoint
			mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
			})

			server := &http.Server{
				Addr:    fmt.Sprintf(":%d", cfg.PrometheusPort),
				Handler: mux,
			}

			log.Printf("Starting Prometheus metrics server on :%d", cfg.PrometheusPort)
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Failed to start Prometheus metrics server: %v", err)
			}
		}()
	} else {
		log.Printf("Prometheus metrics server not started (port not configured)")
	}

	return nil
}

// initStackdriverMetricsExporter initializes the Stackdriver metrics exporter
func initStackdriverMetricsExporter(cfg *config.TracingConfig) error {
	if cfg.StackdriverProjectID == "" {
		return fmt.Errorf("stackdriver project ID is required for Stackdriver metrics exporter")
	}

	// Build options for Stackdriver
	options := stackdriver.Options{
		ProjectID:    cfg.StackdriverProjectID,
		MetricPrefix: cfg.ServiceName,
		OnError: func(err error) {
			log.Printf("Stackdriver metrics exporter error: %v", err)
		},
	}

	// Create the exporter
	se, err := stackdriver.NewExporter(options)
	if err != nil {
		return fmt.Errorf("failed to create Stackdriver metrics exporter: %w", err)
	}

	// Register the exporter with OpenCensus for metrics
	view.RegisterExporter(se)

	log.Printf("Stackdriver metrics exporter initialized with project ID %s", cfg.StackdriverProjectID)
	return nil
}

// initDatadogMetricsExporter initializes the Datadog metrics exporter
func initDatadogMetricsExporter(cfg *config.TracingConfig) error {
	agentAddr := cfg.DatadogAgentAddress
	if agentAddr == "" {
		agentAddr = cfg.AgentEndpoint // Fall back to general agent endpoint
	}

	if agentAddr == "" {
		return fmt.Errorf("datadog agent address is required for Datadog metrics exporter")
	}

	// Build options for Datadog
	options := datadog.Options{
		Service:   cfg.ServiceName,
		TraceAddr: agentAddr, // Used for traces
		StatsAddr: agentAddr, // Used for metrics
		Tags:      []string{"env:prod"},
		OnError: func(err error) {
			log.Printf("Datadog metrics exporter error: %v", err)
		},
	}

	// If API key is provided, add it to the options
	if cfg.DatadogAPIKey != "" {
		options.GlobalTags = map[string]interface{}{
			"api_key": cfg.DatadogAPIKey,
		}
	}

	// Create Datadog exporter
	exporter, err := datadog.NewExporter(options)
	if err != nil {
		return fmt.Errorf("failed to create Datadog metrics exporter: %w", err)
	}

	// Register the exporter with OpenCensus for metrics
	view.RegisterExporter(exporter)

	log.Printf("Datadog metrics exporter initialized with agent address %s", agentAddr)
	return nil
}

// GetHTTPOptions returns options for HTTP client tracing
func GetHTTPOptions() ochttp.Transport {
	return ochttp.Transport{
		Base: nil,
		FormatSpanName: func(req *http.Request) string {
			return fmt.Sprintf("%s %s", req.Method, req.URL.Path)
		},
		StartOptions: trace.StartOptions{
			Sampler: trace.AlwaysSample(),
		},
	}
}

// RegisterHTTPServerViews registers views for HTTP server metrics
func RegisterHTTPServerViews() error {
	return view.Register(
		ochttp.ServerRequestCountView,
		ochttp.ServerRequestBytesView,
		ochttp.ServerResponseBytesView,
		ochttp.ServerLatencyView,
		ochttp.ServerRequestCountByMethod,
		ochttp.ServerResponseCountByStatusCode,
	)
}

// StartSpan starts a new span with the given name and returns a context with the span
func StartSpan(ctx context.Context, name string) (context.Context, *trace.Span) {
	return trace.StartSpan(ctx, name)
}

// StartSpanWithAttributes starts a new span with attributes and returns a context with the span
func StartSpanWithAttributes(ctx context.Context, name string, attrs ...trace.Attribute) (context.Context, *trace.Span) {
	ctx, span := trace.StartSpan(ctx, name)
	span.AddAttributes(attrs...)
	return ctx, span
}

// codecov:ignore:end
