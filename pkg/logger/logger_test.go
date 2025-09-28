package logger

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func captureOutput(f func()) string {
	// Save original output
	oldStdout := os.Stdout

	// Create a pipe to capture output
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Channel to hold the captured output
	outputChan := make(chan string)

	// Start a goroutine to read from the pipe
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outputChan <- buf.String()
	}()

	// Run the function that generates output
	f()

	// Close the writer and restore original stdout
	w.Close()
	os.Stdout = oldStdout

	// Get the captured output
	output := <-outputChan
	return output
}

func TestNewLogger(t *testing.T) {
	// Test creating a new logger
	logger := NewLogger()
	assert.NotNil(t, logger)
	assert.IsType(t, &zerologLogger{}, logger)
}

func TestDebug(t *testing.T) {
	// Configure zerolog to output in a more consistent format for testing
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	// Test debug logging
	output := captureOutput(func() {
		logger := NewLogger()
		logger.Debug("debug message")
	})

	assert.Contains(t, output, "debug message")
	assert.Contains(t, output, `"level":"debug"`)
}

func TestInfo(t *testing.T) {
	// Configure zerolog to output in a more consistent format for testing
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Test info logging
	output := captureOutput(func() {
		logger := NewLogger()
		logger.Info("info message")
	})

	assert.Contains(t, output, "info message")
	assert.Contains(t, output, `"level":"info"`)
}

func TestWarn(t *testing.T) {
	// Configure zerolog to output in a more consistent format for testing
	zerolog.SetGlobalLevel(zerolog.WarnLevel)

	// Test warn logging
	output := captureOutput(func() {
		logger := NewLogger()
		logger.Warn("warn message")
	})

	assert.Contains(t, output, "warn message")
	assert.Contains(t, output, `"level":"warn"`)
}

func TestError(t *testing.T) {
	// Configure zerolog to output in a more consistent format for testing
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)

	// Test error logging
	output := captureOutput(func() {
		logger := NewLogger()
		logger.Error("error message")
	})

	assert.Contains(t, output, "error message")
	assert.Contains(t, output, `"level":"error"`)
}

// We can't easily test Fatal without mocking os.Exit
// and that would require modifying the logger.go file

func TestWithField(t *testing.T) {
	// Configure zerolog to output in a more consistent format for testing
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Test adding fields to logger
	output := captureOutput(func() {
		logger := NewLogger()
		logger = logger.WithField("test_key", "test_value")
		logger.Info("message with field")
	})

	assert.Contains(t, output, "message with field")
	assert.Contains(t, output, `"test_key":"test_value"`)

	// Test multiple fields
	output = captureOutput(func() {
		logger := NewLogger()
		logger = logger.WithField("key1", "value1")
		logger = logger.WithField("key2", "value2")
		logger.Info("message with multiple fields")
	})

	assert.Contains(t, output, "message with multiple fields")
	assert.Contains(t, output, `"key1":"value1"`)
	assert.Contains(t, output, `"key2":"value2"`)

	// Test with different types of values
	output = captureOutput(func() {
		logger := NewLogger()
		logger = logger.WithField("int_field", 123)
		logger = logger.WithField("bool_field", true)
		logger.Info("message with typed fields")
	})

	assert.Contains(t, output, "message with typed fields")
	assert.Contains(t, output, `"int_field":123`)
	assert.Contains(t, output, `"bool_field":true`)
}

func TestWithFieldChaining(t *testing.T) {
	// Configure zerolog to output in a more consistent format for testing
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Test chaining of WithField calls
	output := captureOutput(func() {
		logger := NewLogger().
			WithField("field1", "value1").
			WithField("field2", "value2")
		logger.Info("chained fields")
	})

	assert.Contains(t, output, "chained fields")
	assert.Contains(t, output, `"field1":"value1"`)
	assert.Contains(t, output, `"field2":"value2"`)
}

func TestLogLevelFiltering(t *testing.T) {
	// Test that log levels are properly filtered

	// Set level to Info
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Debug should not be logged
	output := captureOutput(func() {
		logger := NewLogger()
		logger.Debug("debug should be filtered")
	})

	assert.NotContains(t, output, "debug should be filtered")

	// Info should be logged
	output = captureOutput(func() {
		logger := NewLogger()
		logger.Info("info should be logged")
	})

	assert.Contains(t, output, "info should be logged")

	// Set level to Error
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)

	// Info should not be logged
	output = captureOutput(func() {
		logger := NewLogger()
		logger.Info("info should be filtered when level is error")
	})

	assert.NotContains(t, output, "info should be filtered when level is error")

	// Error should be logged
	output = captureOutput(func() {
		logger := NewLogger()
		logger.Error("error should be logged")
	})

	assert.Contains(t, output, "error should be logged")
}

func TestNewLoggerWithLevel(t *testing.T) {
	tests := []struct {
		name          string
		level         string
		expectedLevel zerolog.Level
	}{
		{"debug level", "debug", zerolog.DebugLevel},
		{"info level", "info", zerolog.InfoLevel},
		{"warn level", "warn", zerolog.WarnLevel},
		{"warning level", "warning", zerolog.WarnLevel},
		{"error level", "error", zerolog.ErrorLevel},
		{"fatal level", "fatal", zerolog.FatalLevel},
		{"panic level", "panic", zerolog.PanicLevel},
		{"disabled level", "disabled", zerolog.Disabled},
		{"off level", "off", zerolog.Disabled},
		{"unknown level defaults to info", "unknown", zerolog.InfoLevel},
		{"empty string defaults to info", "", zerolog.InfoLevel},
		{"mixed case", "DEBUG", zerolog.DebugLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create logger with specified level
			logger := NewLoggerWithLevel(tt.level)
			assert.NotNil(t, logger)
			assert.IsType(t, &zerologLogger{}, logger)

			// Check that the global level was set correctly
			assert.Equal(t, tt.expectedLevel, zerolog.GlobalLevel())
		})
	}
}

func TestWithFields(t *testing.T) {
	// Configure zerolog to output in a more consistent format for testing
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Test adding multiple fields at once
	output := captureOutput(func() {
		logger := NewLogger()
		fields := map[string]interface{}{
			"user_id":   123,
			"username":  "testuser",
			"is_active": true,
			"score":     99.5,
		}
		logger = logger.WithFields(fields)
		logger.Info("message with multiple fields")
	})

	assert.Contains(t, output, "message with multiple fields")
	assert.Contains(t, output, `"user_id":123`)
	assert.Contains(t, output, `"username":"testuser"`)
	assert.Contains(t, output, `"is_active":true`)
	assert.Contains(t, output, `"score":99.5`)
}

func TestWithFieldsEmpty(t *testing.T) {
	// Configure zerolog to output in a more consistent format for testing
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Test with empty fields map
	output := captureOutput(func() {
		logger := NewLogger()
		emptyFields := map[string]interface{}{}
		logger = logger.WithFields(emptyFields)
		logger.Info("message with empty fields")
	})

	assert.Contains(t, output, "message with empty fields")
}

func TestWithFieldsNilValues(t *testing.T) {
	// Configure zerolog to output in a more consistent format for testing
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Test with nil values in fields
	output := captureOutput(func() {
		logger := NewLogger()
		fields := map[string]interface{}{
			"nil_field":    nil,
			"string_field": "value",
		}
		logger = logger.WithFields(fields)
		logger.Info("message with nil field")
	})

	assert.Contains(t, output, "message with nil field")
	assert.Contains(t, output, `"nil_field":null`)
	assert.Contains(t, output, `"string_field":"value"`)
}

func TestWithFieldsReturnsNewInstance(t *testing.T) {
	// Test that WithFields returns a new logger instance
	originalLogger := NewLogger()

	fields := map[string]interface{}{
		"field1": "value1",
	}
	newLogger := originalLogger.WithFields(fields)

	// Verify they are different instances
	assert.NotEqual(t, originalLogger, newLogger)
	assert.IsType(t, &zerologLogger{}, newLogger)
}

func TestWithFieldReturnsNewInstance(t *testing.T) {
	// Test that WithField returns a new logger instance
	originalLogger := NewLogger()
	newLogger := originalLogger.WithField("test_field", "test_value")

	// Verify they are different instances
	assert.NotEqual(t, originalLogger, newLogger)
	assert.IsType(t, &zerologLogger{}, newLogger)
}

func TestLoggerMethodsWithEmptyMessages(t *testing.T) {
	// Configure zerolog to output in a more consistent format for testing
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	// Test all log methods with empty messages
	tests := []struct {
		name    string
		logFunc func(Logger)
		level   string
	}{
		{"debug empty", func(l Logger) { l.Debug("") }, "debug"},
		{"info empty", func(l Logger) { l.Info("") }, "info"},
		{"warn empty", func(l Logger) { l.Warn("") }, "warn"},
		{"error empty", func(l Logger) { l.Error("") }, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureOutput(func() {
				logger := NewLogger()
				tt.logFunc(logger)
			})

			assert.Contains(t, output, `"level":"`+tt.level+`"`)
		})
	}
}

func TestCombinedWithFieldAndWithFields(t *testing.T) {
	// Configure zerolog to output in a more consistent format for testing
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Test combining WithField and WithFields
	output := captureOutput(func() {
		logger := NewLogger()
		fields := map[string]interface{}{
			"field1": "value1",
			"field2": "value2",
		}
		logger = logger.WithFields(fields).WithField("field3", "value3")
		logger.Info("combined fields message")
	})

	assert.Contains(t, output, "combined fields message")
	assert.Contains(t, output, `"field1":"value1"`)
	assert.Contains(t, output, `"field2":"value2"`)
	assert.Contains(t, output, `"field3":"value3"`)
}
