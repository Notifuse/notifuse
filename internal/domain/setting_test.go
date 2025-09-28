package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrSettingNotFound_Error(t *testing.T) {
	t.Run("Error message with key", func(t *testing.T) {
		err := &ErrSettingNotFound{Key: "test-key"}
		expected := "setting not found: test-key"
		assert.Equal(t, expected, err.Error())
	})

	t.Run("Error message with empty key", func(t *testing.T) {
		err := &ErrSettingNotFound{Key: ""}
		expected := "setting not found: "
		assert.Equal(t, expected, err.Error())
	})

	t.Run("Error message with special characters", func(t *testing.T) {
		err := &ErrSettingNotFound{Key: "special/key:with-chars_123"}
		expected := "setting not found: special/key:with-chars_123"
		assert.Equal(t, expected, err.Error())
	})

	t.Run("Error message with spaces", func(t *testing.T) {
		err := &ErrSettingNotFound{Key: "key with spaces"}
		expected := "setting not found: key with spaces"
		assert.Equal(t, expected, err.Error())
	})

	t.Run("Error message with unicode characters", func(t *testing.T) {
		err := &ErrSettingNotFound{Key: "clé-avec-accents-éù"}
		expected := "setting not found: clé-avec-accents-éù"
		assert.Equal(t, expected, err.Error())
	})
}

func TestErrSettingNotFound_AsError(t *testing.T) {
	t.Run("Can be used as error interface", func(t *testing.T) {
		var err error = &ErrSettingNotFound{Key: "test-key"}
		assert.Equal(t, "setting not found: test-key", err.Error())
	})

	t.Run("Error type assertion", func(t *testing.T) {
		originalErr := &ErrSettingNotFound{Key: "test-key"}
		var err error = originalErr

		// Test type assertion
		settingErr, ok := err.(*ErrSettingNotFound)
		assert.True(t, ok, "Should be able to assert to *ErrSettingNotFound")
		assert.Equal(t, "test-key", settingErr.Key)
	})
}

func TestErrSettingNotFound_Comparison(t *testing.T) {
	t.Run("Same key errors are equal", func(t *testing.T) {
		err1 := &ErrSettingNotFound{Key: "same-key"}
		err2 := &ErrSettingNotFound{Key: "same-key"}

		assert.Equal(t, err1.Error(), err2.Error())
		assert.Equal(t, err1.Key, err2.Key)
	})

	t.Run("Different key errors are not equal", func(t *testing.T) {
		err1 := &ErrSettingNotFound{Key: "key1"}
		err2 := &ErrSettingNotFound{Key: "key2"}

		assert.NotEqual(t, err1.Error(), err2.Error())
		assert.NotEqual(t, err1.Key, err2.Key)
	})
}

func TestSetting_Struct(t *testing.T) {
	t.Run("Setting struct fields", func(t *testing.T) {
		// Test that Setting struct can be created and accessed
		// This provides some basic coverage for the Setting type
		setting := Setting{
			Key:   "test-key",
			Value: "test-value",
		}

		assert.Equal(t, "test-key", setting.Key)
		assert.Equal(t, "test-value", setting.Value)
		assert.False(t, setting.CreatedAt.IsZero() == false) // CreatedAt should be zero value initially
		assert.False(t, setting.UpdatedAt.IsZero() == false) // UpdatedAt should be zero value initially
	})
}
