package migrations

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
)

// V11Migration implements the migration from version 10.x to 11.0
// Automatically migrates environment variables to database settings for existing deployments
type V11Migration struct{}

// GetMajorVersion returns the major version this migration handles
func (m *V11Migration) GetMajorVersion() float64 {
	return 11.0
}

// HasSystemUpdate indicates if this migration has system-level changes
func (m *V11Migration) HasSystemUpdate() bool {
	return true // Migrates settings to database
}

// HasWorkspaceUpdate indicates if this migration has workspace-level changes
func (m *V11Migration) HasWorkspaceUpdate() bool {
	return false // No workspace-level changes needed
}

// UpdateSystem executes system-level migration changes
// Automatically detects existing installations and migrates env vars to database
func (m *V11Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	// Check if is_installed is already set
	var installedValue string
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = 'is_installed'").Scan(&installedValue)
	if err == nil && installedValue == "true" {
		// Already installed and migrated
		return nil
	}

	// Check if this is an existing installation by looking for users
	var userCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		return fmt.Errorf("failed to check for existing users: %w", err)
	}

	// If no users exist, this is a fresh installation - skip migration
	if userCount == 0 {
		return nil
	}

	// This is an existing installation - migrate env vars to database
	now := time.Now().UTC()

	// Helper function to insert/update setting
	upsertSetting := func(key, value string) error {
		if value == "" {
			return nil // Skip empty values
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO settings (key, value, created_at, updated_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (key) DO UPDATE SET
				value = EXCLUDED.value,
				updated_at = EXCLUDED.updated_at
		`, key, value, now, now)
		return err
	}

	// 1. Migrate ROOT_EMAIL
	if cfg.RootEmail != "" {
		if err := upsertSetting("root_email", cfg.RootEmail); err != nil {
			return fmt.Errorf("failed to migrate root_email: %w", err)
		}
	}

	// 2. Migrate API_ENDPOINT
	if cfg.APIEndpoint != "" {
		if err := upsertSetting("api_endpoint", cfg.APIEndpoint); err != nil {
			return fmt.Errorf("failed to migrate api_endpoint: %w", err)
		}
	}

	// 3. Migrate PASETO keys (encrypted)
	if len(cfg.Security.PasetoPrivateKeyBytes) > 0 {
		// Base64 encode the key bytes first (to match what was in env var)
		privateKeyBase64 := base64.StdEncoding.EncodeToString(cfg.Security.PasetoPrivateKeyBytes)

		// Then encrypt the base64 string
		encrypted, err := crypto.EncryptString(privateKeyBase64, cfg.Security.SecretKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt PASETO private key: %w", err)
		}

		if err := upsertSetting("encrypted_paseto_private_key", encrypted); err != nil {
			return fmt.Errorf("failed to migrate encrypted_paseto_private_key: %w", err)
		}
	}

	if len(cfg.Security.PasetoPublicKeyBytes) > 0 {
		// Base64 encode the key bytes first (to match what was in env var)
		publicKeyBase64 := base64.StdEncoding.EncodeToString(cfg.Security.PasetoPublicKeyBytes)

		// Then encrypt the base64 string
		encrypted, err := crypto.EncryptString(publicKeyBase64, cfg.Security.SecretKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt PASETO public key: %w", err)
		}

		if err := upsertSetting("encrypted_paseto_public_key", encrypted); err != nil {
			return fmt.Errorf("failed to migrate encrypted_paseto_public_key: %w", err)
		}
	}

	// 4. Migrate SMTP settings
	if cfg.SMTP.Host != "" {
		if err := upsertSetting("smtp_host", cfg.SMTP.Host); err != nil {
			return fmt.Errorf("failed to migrate smtp_host: %w", err)
		}
	}

	if cfg.SMTP.Port > 0 {
		if err := upsertSetting("smtp_port", strconv.Itoa(cfg.SMTP.Port)); err != nil {
			return fmt.Errorf("failed to migrate smtp_port: %w", err)
		}
	}

	if cfg.SMTP.Username != "" {
		if err := upsertSetting("smtp_username", cfg.SMTP.Username); err != nil {
			return fmt.Errorf("failed to migrate smtp_username: %w", err)
		}
	}

	if cfg.SMTP.Password != "" {
		// Encrypt SMTP password
		encrypted, err := crypto.EncryptString(cfg.SMTP.Password, cfg.Security.SecretKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt SMTP password: %w", err)
		}
		if err := upsertSetting("encrypted_smtp_password", encrypted); err != nil {
			return fmt.Errorf("failed to migrate encrypted_smtp_password: %w", err)
		}
	}

	if cfg.SMTP.FromEmail != "" {
		if err := upsertSetting("smtp_from_email", cfg.SMTP.FromEmail); err != nil {
			return fmt.Errorf("failed to migrate smtp_from_email: %w", err)
		}
	}

	if cfg.SMTP.FromName != "" {
		if err := upsertSetting("smtp_from_name", cfg.SMTP.FromName); err != nil {
			return fmt.Errorf("failed to migrate smtp_from_name: %w", err)
		}
	}

	// 5. Mark as installed
	if err := upsertSetting("is_installed", "true"); err != nil {
		return fmt.Errorf("failed to set is_installed: %w", err)
	}

	return nil
}

// UpdateWorkspace executes workspace-level migration changes (none for v11)
func (m *V11Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	return nil
}

// init registers this migration with the default registry
func init() {
	Register(&V11Migration{})
}
