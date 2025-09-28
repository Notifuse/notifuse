package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

const (
	LastCronRunKey = "last_cron_run"
)

// SQLSettingRepository is a SQL implementation of the SettingRepository interface
type SQLSettingRepository struct {
	systemDB *sql.DB
}

// NewSQLSettingRepository creates a new SQLSettingRepository
func NewSQLSettingRepository(db *sql.DB) *SQLSettingRepository {
	return &SQLSettingRepository{
		systemDB: db,
	}
}

// Get retrieves a setting by key
func (r *SQLSettingRepository) Get(ctx context.Context, key string) (*domain.Setting, error) {
	var setting domain.Setting
	err := r.systemDB.QueryRowContext(ctx,
		"SELECT key, value, created_at, updated_at FROM settings WHERE key = $1",
		key,
	).Scan(&setting.Key, &setting.Value, &setting.CreatedAt, &setting.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &domain.ErrSettingNotFound{Key: key}
		}
		return nil, err
	}

	return &setting, nil
}

// Set creates or updates a setting
func (r *SQLSettingRepository) Set(ctx context.Context, key, value string) error {
	now := time.Now()

	_, err := r.systemDB.ExecContext(ctx, `
		INSERT INTO settings (key, value, created_at, updated_at) 
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (key) 
		DO UPDATE SET 
			value = EXCLUDED.value,
			updated_at = EXCLUDED.updated_at
	`, key, value, now, now)

	return err
}

// Delete removes a setting by key
func (r *SQLSettingRepository) Delete(ctx context.Context, key string) error {
	result, err := r.systemDB.ExecContext(ctx,
		"DELETE FROM settings WHERE key = $1",
		key,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return &domain.ErrSettingNotFound{Key: key}
	}

	return nil
}

// List retrieves all settings
func (r *SQLSettingRepository) List(ctx context.Context) ([]*domain.Setting, error) {
	rows, err := r.systemDB.QueryContext(ctx,
		"SELECT key, value, created_at, updated_at FROM settings ORDER BY key",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []*domain.Setting
	for rows.Next() {
		setting := &domain.Setting{}
		err := rows.Scan(&setting.Key, &setting.Value, &setting.CreatedAt, &setting.UpdatedAt)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return settings, nil
}

// SetLastCronRun updates the last cron execution timestamp to NOW()
func (r *SQLSettingRepository) SetLastCronRun(ctx context.Context) error {
	now := time.Now()
	return r.Set(ctx, LastCronRunKey, now.Format(time.RFC3339))
}

// GetLastCronRun retrieves the last cron execution timestamp
func (r *SQLSettingRepository) GetLastCronRun(ctx context.Context) (*time.Time, error) {
	setting, err := r.Get(ctx, LastCronRunKey)
	if err != nil {
		if _, ok := err.(*domain.ErrSettingNotFound); ok {
			// If no last cron run is found, return nil (no error)
			return nil, nil
		}
		return nil, err
	}

	// Parse the timestamp
	timestamp, err := time.Parse(time.RFC3339, setting.Value)
	if err != nil {
		return nil, err
	}

	return &timestamp, nil
}
