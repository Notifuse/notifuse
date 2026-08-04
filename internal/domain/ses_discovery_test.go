package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Requests are validated from raw JSON, not struct literals: a struct literal cannot express a
// field that is absent, which is exactly the case these rules exist to catch.
func decodeRef(t *testing.T, body string) domain.SESCredentialsRef {
	t.Helper()
	var ref domain.SESCredentialsRef
	require.NoError(t, json.Unmarshal([]byte(body), &ref))
	return ref
}

func TestSESCredentialsRef_Validate(t *testing.T) {
	t.Run("saved integration mode", func(t *testing.T) {
		ref := decodeRef(t, `{"workspace_id":"ws","integration_id":"int-1"}`)
		assert.NoError(t, ref.Validate())
		assert.True(t, ref.UsesSavedIntegration())
	})

	t.Run("inline credentials mode", func(t *testing.T) {
		ref := decodeRef(t, `{"workspace_id":"ws","region":"eu-west-3","access_key":"AKIA","secret_key":"s"}`)
		assert.NoError(t, ref.Validate())
		assert.False(t, ref.UsesSavedIntegration())
	})

	t.Run("workspace is always required", func(t *testing.T) {
		ref := decodeRef(t, `{"integration_id":"int-1"}`)
		err := ref.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id")
	})

	t.Run("neither mode supplied", func(t *testing.T) {
		ref := decodeRef(t, `{"workspace_id":"ws"}`)
		err := ref.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "integration_id")
		assert.Contains(t, err.Error(), "secret_key")
	})

	t.Run("partial inline credentials are refused", func(t *testing.T) {
		for _, body := range []string{
			`{"workspace_id":"ws","region":"eu-west-3"}`,
			`{"workspace_id":"ws","region":"eu-west-3","access_key":"AKIA"}`,
			`{"workspace_id":"ws","access_key":"AKIA","secret_key":"s"}`,
		} {
			ref := decodeRef(t, body)
			assert.Error(t, ref.Validate(), "expected %s to be rejected", body)
		}
	})

	t.Run("region is never taken on trust", func(t *testing.T) {
		// It is interpolated into the AWS endpoint host.
		for _, region := range []string{"evil.example.com", "../", "US-EAST-1", "not-a-region"} {
			ref := domain.SESCredentialsRef{
				WorkspaceID: "ws", Region: region, AccessKey: "AKIA", SecretKey: "s",
			}
			err := ref.Validate()
			require.Error(t, err, "expected %q to be rejected", region)
			assert.Contains(t, err.Error(), "region")
		}
	})
}

func TestVerifySESTenantRequest_Validate(t *testing.T) {
	var req domain.VerifySESTenantRequest
	require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"ws","integration_id":"int-1"}`), &req))
	err := req.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant_name")

	require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"ws","integration_id":"int-1","tenant_name":"t"}`), &req))
	assert.NoError(t, req.Validate())
}

func TestEnableSESTenantIsolationRequest_Validate(t *testing.T) {
	var req domain.EnableSESTenantIsolationRequest

	// Provisioning creates a billable resource and writes derived state back, so it must never
	// run against credentials typed into an unsaved form.
	require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"ws"}`), &req))
	err := req.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "integration_id")

	require.NoError(t, json.Unmarshal([]byte(`{"workspace_id":"ws","integration_id":"int-1"}`), &req))
	assert.NoError(t, req.Validate())
}
