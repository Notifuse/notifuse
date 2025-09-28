package domain

import (
	"context"
	"time"
)

//go:generate mockgen -destination mocks/mock_setting_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain SettingRepository

// Setting represents a system setting
type Setting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SettingRepository defines the interface for setting-related database operations
type SettingRepository interface {
	// Get retrieves a setting by key
	Get(ctx context.Context, key string) (*Setting, error)

	// Set creates or updates a setting
	Set(ctx context.Context, key, value string) error

	// Delete removes a setting by key
	Delete(ctx context.Context, key string) error

	// List retrieves all settings
	List(ctx context.Context) ([]*Setting, error)

	// SetLastCronRun updates the last cron execution timestamp
	SetLastCronRun(ctx context.Context) error

	// GetLastCronRun retrieves the last cron execution timestamp
	GetLastCronRun(ctx context.Context) (*time.Time, error)
}

// ErrSettingNotFound is returned when a setting is not found
type ErrSettingNotFound struct {
	Key string
}

func (e *ErrSettingNotFound) Error() string {
	return "setting not found: " + e.Key
}
