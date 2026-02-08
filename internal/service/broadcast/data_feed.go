package broadcast

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

const (
	// DefaultFetchTimeout is the default timeout for data feed requests
	DefaultFetchTimeout = 10 * time.Second

	// MaxResponseSize is the maximum response size (10MB)
	MaxResponseSize = 10 * 1024 * 1024

	// UserAgent is the User-Agent header sent with requests
	UserAgent = "Notifuse/1.0 DataFeedFetcher"
)

//go:generate mockgen -destination=./mocks/mock_data_feed_fetcher.go -package=mocks github.com/Notifuse/notifuse/internal/service/broadcast DataFeedFetcher

// DataFeedFetcher handles external data fetching for broadcasts
type DataFeedFetcher interface {
	// FetchGlobal fetches global data from a configured endpoint
	// Returns nil, nil if settings are nil or disabled
	FetchGlobal(ctx context.Context, settings *domain.GlobalFeedSettings,
		payload *domain.GlobalFeedRequestPayload) (map[string]interface{}, error)

	// FetchRecipient fetches per-recipient data from a configured endpoint
	// Returns nil, nil if settings are nil or disabled
	// Supports retry logic for 5xx errors and 408/429 status codes
	FetchRecipient(ctx context.Context, settings *domain.RecipientFeedSettings,
		payload *domain.RecipientFeedRequestPayload) (map[string]interface{}, error)
}

// dataFeedFetcher implements the DataFeedFetcher interface
type dataFeedFetcher struct {
	httpClient *http.Client
	logger     logger.Logger
}

// NewDataFeedFetcher creates a new DataFeedFetcher instance
func NewDataFeedFetcher(log logger.Logger) DataFeedFetcher {
	return &dataFeedFetcher{
		httpClient: &http.Client{
			// Base timeout; will be overridden per-request
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		logger: log,
	}
}

// FetchGlobal fetches global data from a configured endpoint
func (f *dataFeedFetcher) FetchGlobal(ctx context.Context, settings *domain.GlobalFeedSettings,
	payload *domain.GlobalFeedRequestPayload) (map[string]interface{}, error) {

	// Return early if settings are nil or disabled
	if settings == nil || !settings.Enabled {
		f.logger.WithFields(map[string]interface{}{
			"settings_nil": settings == nil,
			"enabled":      settings != nil && settings.Enabled,
		}).Debug("Global feed fetch skipped: not enabled")
		return nil, nil
	}

	// Determine timeout
	timeout := time.Duration(settings.GetTimeout()) * time.Second

	// Create a context with timeout
	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Prepare payload (use empty struct if nil)
	var payloadBytes []byte
	var err error
	if payload != nil {
		payloadBytes, err = json.Marshal(payload)
	} else {
		payloadBytes, err = json.Marshal(struct{}{})
	}
	if err != nil {
		f.logger.WithFields(map[string]interface{}{
			"url":   settings.URL,
			"error": err.Error(),
		}).Error("Failed to marshal request payload")
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	f.logger.WithFields(map[string]interface{}{
		"url":     settings.URL,
		"timeout": timeout.String(),
	}).Debug("Fetching global feed data")

	// Create request
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodPost, settings.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		f.logger.WithFields(map[string]interface{}{
			"url":   settings.URL,
			"error": err.Error(),
		}).Error("Failed to create request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	// Add custom headers
	for _, header := range settings.Headers {
		req.Header.Set(header.Name, header.Value)
	}

	// Execute request
	resp, err := f.httpClient.Do(req)
	if err != nil {
		// Check if it's a timeout error
		if fetchCtx.Err() == context.DeadlineExceeded {
			f.logger.WithFields(map[string]interface{}{
				"url":     settings.URL,
				"timeout": timeout.String(),
			}).Warn("Global feed request timed out")
			return nil, fmt.Errorf("request timeout after %s", timeout.String())
		}
		if ctx.Err() != nil {
			f.logger.WithFields(map[string]interface{}{
				"url":   settings.URL,
				"error": ctx.Err().Error(),
			}).Warn("Global feed request cancelled")
			return nil, fmt.Errorf("request cancelled: %w", ctx.Err())
		}
		f.logger.WithFields(map[string]interface{}{
			"url":   settings.URL,
			"error": err.Error(),
		}).Error("Failed to execute request")
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read error body for logging
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		bodyStr := string(bodyBytes)
		if bodyStr == "" {
			bodyStr = "(empty)"
		}

		f.logger.WithFields(map[string]interface{}{
			"url":         settings.URL,
			"status_code": resp.StatusCode,
			"body":        bodyStr,
		}).Error("Global feed returned HTTP error")
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, getHTTPStatusDescription(resp.StatusCode))
	}

	// Read response body with size limit
	limitedReader := io.LimitReader(resp.Body, MaxResponseSize)
	responseBody, err := io.ReadAll(limitedReader)
	if err != nil {
		f.logger.WithFields(map[string]interface{}{
			"url":   settings.URL,
			"error": err.Error(),
		}).Error("Failed to read response body")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse JSON response directly into map (accept any valid JSON object)
	var result map[string]interface{}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		f.logger.WithFields(map[string]interface{}{
			"url":           settings.URL,
			"error":         err.Error(),
			"response_size": len(responseBody),
		}).Error("Failed to parse JSON response")
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	if result == nil {
		result = make(map[string]interface{})
	}

	// Add metadata
	result["_success"] = true
	result["_fetched_at"] = time.Now().UTC().Format(time.RFC3339)

	f.logger.WithFields(map[string]interface{}{
		"url":        settings.URL,
		"data_keys":  len(result) - 2, // Exclude _success and _fetched_at
		"fetched_at": result["_fetched_at"],
	}).Info("Global feed data fetched successfully")

	return result, nil
}

// FetchRecipient fetches per-recipient data from a configured endpoint
func (f *dataFeedFetcher) FetchRecipient(ctx context.Context, settings *domain.RecipientFeedSettings,
	payload *domain.RecipientFeedRequestPayload) (map[string]interface{}, error) {

	// Return early if settings are nil or disabled
	if settings == nil || !settings.Enabled {
		f.logger.WithFields(map[string]interface{}{
			"settings_nil": settings == nil,
			"enabled":      settings != nil && settings.Enabled,
		}).Debug("Recipient feed fetch skipped: not enabled")
		return nil, nil
	}

	// Determine timeout and retry settings
	timeout := time.Duration(settings.GetTimeout()) * time.Second
	maxRetries := settings.GetMaxRetries()
	retryDelay := time.Duration(settings.GetRetryDelay()) * time.Millisecond

	// Prepare payload (use empty struct if nil)
	var payloadBytes []byte
	var err error
	if payload != nil {
		payloadBytes, err = json.Marshal(payload)
	} else {
		payloadBytes, err = json.Marshal(struct{}{})
	}
	if err != nil {
		f.logger.WithFields(map[string]interface{}{
			"url":   settings.URL,
			"error": err.Error(),
		}).Error("Failed to marshal request payload")
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	f.logger.WithFields(map[string]interface{}{
		"url":         settings.URL,
		"timeout":     timeout.String(),
		"max_retries": maxRetries,
		"retry_delay": retryDelay.String(),
	}).Debug("Fetching recipient feed data")

	// Execute request with retry logic
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			f.logger.WithFields(map[string]interface{}{
				"url":     settings.URL,
				"attempt": attempt + 1,
			}).Debug("Retrying recipient feed request")
			time.Sleep(retryDelay)
		}

		result, statusCode, err := f.doRecipientRequest(ctx, settings, payloadBytes, timeout)
		if err != nil {
			lastErr = err
			// Check if error is retryable
			if !f.shouldRetry(statusCode, err, attempt, maxRetries) {
				return nil, err
			}
			continue
		}

		return result, nil
	}

	return nil, lastErr
}

// doRecipientRequest performs a single recipient feed request
func (f *dataFeedFetcher) doRecipientRequest(ctx context.Context, settings *domain.RecipientFeedSettings,
	payloadBytes []byte, timeout time.Duration) (map[string]interface{}, int, error) {

	// Create a context with timeout
	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create request
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodPost, settings.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		f.logger.WithFields(map[string]interface{}{
			"url":   settings.URL,
			"error": err.Error(),
		}).Error("Failed to create request")
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	// Add custom headers
	for _, header := range settings.Headers {
		req.Header.Set(header.Name, header.Value)
	}

	// Execute request
	resp, err := f.httpClient.Do(req)
	if err != nil {
		// Check if it's a timeout error
		if fetchCtx.Err() == context.DeadlineExceeded {
			f.logger.WithFields(map[string]interface{}{
				"url":     settings.URL,
				"timeout": timeout.String(),
			}).Warn("Recipient feed request timed out")
			return nil, 0, fmt.Errorf("request timeout after %s", timeout.String())
		}
		if ctx.Err() != nil {
			f.logger.WithFields(map[string]interface{}{
				"url":   settings.URL,
				"error": ctx.Err().Error(),
			}).Warn("Recipient feed request cancelled")
			return nil, 0, fmt.Errorf("request cancelled: %w", ctx.Err())
		}
		f.logger.WithFields(map[string]interface{}{
			"url":   settings.URL,
			"error": err.Error(),
		}).Error("Failed to execute request")
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read error body for logging
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		bodyStr := string(bodyBytes)
		if bodyStr == "" {
			bodyStr = "(empty)"
		}

		f.logger.WithFields(map[string]interface{}{
			"url":         settings.URL,
			"status_code": resp.StatusCode,
			"body":        bodyStr,
		}).Error("Recipient feed returned HTTP error")
		return nil, resp.StatusCode, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, getHTTPStatusDescription(resp.StatusCode))
	}

	// Read response body with size limit
	limitedReader := io.LimitReader(resp.Body, MaxResponseSize)
	responseBody, err := io.ReadAll(limitedReader)
	if err != nil {
		f.logger.WithFields(map[string]interface{}{
			"url":   settings.URL,
			"error": err.Error(),
		}).Error("Failed to read response body")
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse JSON response directly into map (accept any valid JSON object)
	var result map[string]interface{}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		f.logger.WithFields(map[string]interface{}{
			"url":           settings.URL,
			"error":         err.Error(),
			"response_size": len(responseBody),
		}).Error("Failed to parse JSON response")
		return nil, resp.StatusCode, fmt.Errorf("invalid JSON response: %w", err)
	}

	if result == nil {
		result = make(map[string]interface{})
	}

	// Add metadata
	result["_success"] = true
	result["_fetched_at"] = time.Now().UTC().Format(time.RFC3339)

	f.logger.WithFields(map[string]interface{}{
		"url":        settings.URL,
		"data_keys":  len(result) - 2, // Exclude _success and _fetched_at
		"fetched_at": result["_fetched_at"],
	}).Info("Recipient feed data fetched successfully")

	return result, resp.StatusCode, nil
}

// shouldRetry determines if a request should be retried based on status code and error
func (f *dataFeedFetcher) shouldRetry(statusCode int, err error, attempt, maxRetries int) bool {
	// Don't retry if we've exhausted all retries
	if attempt >= maxRetries {
		return false
	}

	// Retry on 5xx errors
	if statusCode >= 500 && statusCode < 600 {
		return true
	}

	// Retry on 408 Request Timeout
	if statusCode == http.StatusRequestTimeout {
		return true
	}

	// Retry on 429 Too Many Requests
	if statusCode == http.StatusTooManyRequests {
		return true
	}

	// Retry on network/connection errors
	if isRetryableError(err) {
		return true
	}

	// Don't retry on other 4xx errors
	return false
}

// getHTTPStatusDescription returns a human-readable description for common HTTP status codes
func getHTTPStatusDescription(statusCode int) string {
	descriptions := map[int]string{
		http.StatusBadRequest:          "Bad Request",
		http.StatusUnauthorized:        "Unauthorized",
		http.StatusForbidden:           "Forbidden",
		http.StatusNotFound:            "Not Found",
		http.StatusMethodNotAllowed:    "Method Not Allowed",
		http.StatusRequestTimeout:      "Request Timeout",
		http.StatusConflict:            "Conflict",
		http.StatusGone:                "Gone",
		http.StatusUnprocessableEntity: "Unprocessable Entity",
		http.StatusTooManyRequests:     "Too Many Requests",
		http.StatusInternalServerError: "Internal Server Error",
		http.StatusBadGateway:          "Bad Gateway",
		http.StatusServiceUnavailable:  "Service Unavailable",
		http.StatusGatewayTimeout:      "Gateway Timeout",
	}

	if desc, ok := descriptions[statusCode]; ok {
		return desc
	}

	// Use standard library description
	text := http.StatusText(statusCode)
	if text != "" {
		return text
	}

	return "Unknown Error"
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	retryablePatterns := []string{
		"connection reset",
		"connection refused",
		"timeout",
		"temporary failure",
		"no such host",
		"i/o timeout",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}

	return false
}
