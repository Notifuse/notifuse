package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestPreserveFileManagerSecret(t *testing.T) {
	stored := domain.FileManagerSettings{
		Endpoint:           "https://s3.example.com",
		Bucket:             "assets",
		AccessKey:          "AK",
		EncryptedSecretKey: "stored-ciphertext",
	}

	t.Run("same access key and host keep the stored secret", func(t *testing.T) {
		updated := domain.FileManagerSettings{Endpoint: "https://s3.example.com/", Bucket: "other", AccessKey: "AK"}
		require.NoError(t, preserveFileManagerSecret(&updated, stored))
		assert.Equal(t, "stored-ciphertext", updated.EncryptedSecretKey)
	})

	t.Run("a typed secret replaces it", func(t *testing.T) {
		updated := domain.FileManagerSettings{Endpoint: "https://elsewhere.example.com", Bucket: "b", AccessKey: "AK2", SecretKey: "new"}
		require.NoError(t, preserveFileManagerSecret(&updated, stored))
		assert.Empty(t, updated.EncryptedSecretKey)
		assert.Equal(t, "new", updated.SecretKey)
	})

	t.Run("another host needs the secret again", func(t *testing.T) {
		updated := domain.FileManagerSettings{Endpoint: "https://collector.example.net", Bucket: "assets", AccessKey: "AK"}
		err := preserveFileManagerSecret(&updated, stored)
		var validation domain.ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Empty(t, updated.EncryptedSecretKey)
	})

	t.Run("another access key needs the secret again", func(t *testing.T) {
		updated := domain.FileManagerSettings{Endpoint: "https://s3.example.com", Bucket: "assets", AccessKey: "AK2"}
		err := preserveFileManagerSecret(&updated, stored)
		var validation domain.ValidationError
		require.ErrorAs(t, err, &validation)
	})

	t.Run("switching the file manager off drops the secret", func(t *testing.T) {
		updated := domain.FileManagerSettings{}
		require.NoError(t, preserveFileManagerSecret(&updated, stored))
		assert.Empty(t, updated.EncryptedSecretKey)
	})

	t.Run("nothing stored, nothing to keep", func(t *testing.T) {
		updated := domain.FileManagerSettings{Endpoint: "https://collector.example.net", Bucket: "b", AccessKey: "AK"}
		require.NoError(t, preserveFileManagerSecret(&updated, domain.FileManagerSettings{}))
		assert.Empty(t, updated.EncryptedSecretKey)
	})
}

func TestValidateChangedFileManagerEndpoint(t *testing.T) {
	stored := domain.FileManagerSettings{Endpoint: "s3.legacy.example.com"}

	assert.NoError(t, validateChangedFileManagerEndpoint(domain.FileManagerSettings{Endpoint: "s3.legacy.example.com"}, stored),
		"an endpoint stored before the rule does not block other saves")
	assert.NoError(t, validateChangedFileManagerEndpoint(domain.FileManagerSettings{}, stored), "switching the file manager off")
	assert.NoError(t, validateChangedFileManagerEndpoint(domain.FileManagerSettings{Endpoint: "https://s3.example.com"}, stored))

	for _, endpoint := range []string{"s3.example.com", "ftp://files.example.com", "https://user:pw@s3.example.com"} {
		err := validateChangedFileManagerEndpoint(domain.FileManagerSettings{Endpoint: endpoint}, stored)
		var validation domain.ValidationError
		assert.ErrorAs(t, err, &validation, endpoint)
	}
}
