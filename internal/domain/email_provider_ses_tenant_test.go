package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SESv2Client must stay satisfiable by the real SDK client: this is what catches a signature
// drift on the next SDK bump, when nothing else would until runtime.
var _ domain.SESv2Client = (*sesv2.Client)(nil)

func TestAmazonSESSettings_Validate_SendingContext(t *testing.T) {
	base := func() domain.AmazonSESSettings {
		return domain.AmazonSESSettings{
			Region:    "eu-west-3",
			AccessKey: "AKIAEXAMPLE",
			SecretKey: "secret",
		}
	}

	t.Run("empty tenant fields are valid", func(t *testing.T) {
		s := base()
		require.NoError(t, s.Validate("passphrase"))
	})

	t.Run("valid names accepted", func(t *testing.T) {
		s := base()
		s.ConfigurationSetName = "notifuse-a3f1_set-1"
		s.TenantName = "team-acme"
		require.NoError(t, s.Validate("passphrase"))
	})

	t.Run("name over 64 chars rejected", func(t *testing.T) {
		s := base()
		s.TenantName = strings.Repeat("a", 65)
		err := s.Validate("passphrase")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tenant name")
	})

	t.Run("names with illegal characters rejected", func(t *testing.T) {
		for _, name := range []string{"my set", "my.set", "my/set", "my:set", ""} {
			if name == "" {
				continue // empty means "not set", covered above
			}
			s := base()
			s.ConfigurationSetName = name
			err := s.Validate("passphrase")
			require.Error(t, err, "expected %q to be rejected", name)
			assert.Contains(t, err.Error(), "configuration set name")
		}
	})

	t.Run("tenant name alone still requires a region", func(t *testing.T) {
		s := domain.AmazonSESSettings{TenantName: "team-acme"}
		err := s.Validate("passphrase")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "region is required")
	})

	t.Run("isolation flag alone still requires a region", func(t *testing.T) {
		s := domain.AmazonSESSettings{TenantIsolationEnabled: true}
		err := s.Validate("passphrase")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "region is required")
	})
}

func TestAmazonSESSettings_Validate_RejectsManagedAndManualTenant(t *testing.T) {
	s := domain.AmazonSESSettings{
		Region:                 "eu-west-3",
		AccessKey:              "AKIAEXAMPLE",
		SecretKey:              "secret",
		TenantIsolationEnabled: true,
		TenantName:             "mine",
	}

	err := s.Validate("passphrase")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant isolation")
	assert.Contains(t, err.Error(), "tenant name")
}

func TestAmazonSESSettings_JSONRoundTrip_OmitsEmptySendingContext(t *testing.T) {
	t.Run("absent fields emit no keys", func(t *testing.T) {
		s := domain.AmazonSESSettings{Region: "eu-west-3", AccessKey: "AKIAEXAMPLE"}

		raw, err := json.Marshal(s)
		require.NoError(t, err)

		// Existing stored integrations must not change shape on the next save.
		for _, key := range []string{
			"tenant_isolation_enabled", "configuration_set_name", "tenant_name",
			"managed_configuration_set", "managed_tenant_name",
		} {
			assert.NotContains(t, string(raw), key)
		}
	})

	t.Run("stored blob unmarshals intact", func(t *testing.T) {
		raw := `{
			"region": "eu-west-3",
			"access_key": "AKIAEXAMPLE",
			"tenant_isolation_enabled": true,
			"configuration_set_name": "custom-set",
			"managed_configuration_set": "notifuse-abc",
			"managed_tenant_name": "notifuse-abc"
		}`

		var s domain.AmazonSESSettings
		require.NoError(t, json.Unmarshal([]byte(raw), &s))

		assert.True(t, s.TenantIsolationEnabled)
		assert.Equal(t, "custom-set", s.ConfigurationSetName)
		assert.Equal(t, "notifuse-abc", s.ManagedConfigurationSet)
		assert.Equal(t, "notifuse-abc", s.ManagedTenantName)
	})
}

func TestAmazonSESSettings_Resolution(t *testing.T) {
	t.Run("override wins over managed", func(t *testing.T) {
		s := domain.AmazonSESSettings{
			ConfigurationSetName:    "custom-set",
			ManagedConfigurationSet: "notifuse-abc",
			TenantName:              "team-acme",
			ManagedTenantName:       "notifuse-abc",
		}
		assert.Equal(t, "custom-set", s.ResolveConfigurationSet())
		assert.Equal(t, "team-acme", s.ResolveTenant())
	})

	t.Run("managed used when no override and isolation is on", func(t *testing.T) {
		s := domain.AmazonSESSettings{
			TenantIsolationEnabled:  true,
			ManagedConfigurationSet: "notifuse-abc",
			ManagedTenantName:       "notifuse-abc",
		}
		assert.Equal(t, "notifuse-abc", s.ResolveConfigurationSet())
		assert.Equal(t, "notifuse-abc", s.ResolveTenant())
	})

	// The switch has to work in both directions. Without this, an operator who turns isolation
	// off still has every message scoped to a tenant they believe is disabled, and the only way
	// to stop is to delete the integration.
	t.Run("switching isolation off stops sends using the managed tenant", func(t *testing.T) {
		s := domain.AmazonSESSettings{
			TenantIsolationEnabled:  false,
			ManagedConfigurationSet: "notifuse-abc",
			ManagedTenantName:       "notifuse-abc",
		}
		assert.Empty(t, s.ResolveTenant())
		// The configuration set is still ours and still needed for event tracking.
		assert.Equal(t, "notifuse-abc", s.ResolveConfigurationSet())
		// But the tenant still exists in AWS, so teardown must still find it.
		assert.Equal(t, "notifuse-abc", s.KnownTenant())
		assert.True(t, s.OwnsManagedTenant())
	})

	t.Run("a manual tenant is used regardless of the switch", func(t *testing.T) {
		s := domain.AmazonSESSettings{TenantName: "team-acme"}
		assert.Equal(t, "team-acme", s.ResolveTenant())
		assert.Equal(t, "team-acme", s.KnownTenant())
		assert.False(t, s.OwnsManagedTenant(), "an operator's tenant is not ours to delete")
	})

	t.Run("empty when neither set", func(t *testing.T) {
		s := domain.AmazonSESSettings{}
		assert.Empty(t, s.ResolveConfigurationSet())
		assert.Empty(t, s.ResolveTenant())
	})
}

func TestIsSESAccessDenied(t *testing.T) {
	t.Run("matches the code AWS actually returns", func(t *testing.T) {
		// AccessDeniedException is a COMMON SES error with no generated Go type, so this can
		// only ever be matched on the code string.
		for _, code := range []string{"AccessDeniedException", "AccessDenied", "NotAuthorized"} {
			err := &smithy.GenericAPIError{Code: code, Message: "nope"}
			assert.True(t, domain.IsSESAccessDenied(err), "expected %s to be a denial", code)
		}
	})

	t.Run("matches through wrapping", func(t *testing.T) {
		wrapped := errors.New("list tenants: " + (&smithy.GenericAPIError{Code: "AccessDeniedException"}).Error())
		assert.False(t, domain.IsSESAccessDenied(wrapped), "string wrapping is not unwrapping")

		properly := &smithy.OperationError{
			ServiceID:     "SESv2",
			OperationName: "ListTenants",
			Err:           &smithy.GenericAPIError{Code: "AccessDeniedException"},
		}
		assert.True(t, domain.IsSESAccessDenied(properly))
	})

	t.Run("sentinel and unrelated errors", func(t *testing.T) {
		assert.True(t, domain.IsSESAccessDenied(domain.ErrSESAccessDenied))
		assert.False(t, domain.IsSESAccessDenied(nil))
		assert.False(t, domain.IsSESAccessDenied(errors.New("boom")))
		assert.False(t, domain.IsSESAccessDenied(&smithy.GenericAPIError{Code: "NotFoundException"}))
	})
}

func TestSESResourceARN(t *testing.T) {
	tenantARN := "arn:aws:ses:eu-west-3:123456789012:tenant/notifuse-abc/t-1"

	assert.Equal(t,
		"arn:aws:ses:eu-west-3:123456789012:configuration-set/notifuse-abc",
		domain.SESResourceARN(tenantARN, "configuration-set", "notifuse-abc"))

	assert.Equal(t,
		"arn:aws:ses:eu-west-3:123456789012:identity/acme.com",
		domain.SESResourceARN(tenantARN, "identity", "acme.com"))

	assert.Empty(t, domain.SESResourceARN("not-an-arn", "identity", "acme.com"))
}

func TestSESResourceNameFromARN(t *testing.T) {
	assert.Equal(t, "notifuse-abc",
		domain.SESResourceNameFromARN("arn:aws:ses:eu-west-3:123456789012:configuration-set/notifuse-abc"))

	// A suffix comparison would call this a match for "notifuse-abc"; a parsed one does not.
	assert.Equal(t, "prod-notifuse-abc",
		domain.SESResourceNameFromARN("arn:aws:ses:eu-west-3:123456789012:configuration-set/prod-notifuse-abc"))

	assert.Empty(t, domain.SESResourceNameFromARN("arn:aws:ses:eu-west-3:123456789012:configuration-set"))
	assert.Empty(t, domain.SESResourceNameFromARN("garbage"))
}

func TestIsValidSESRegion(t *testing.T) {
	assert.True(t, domain.IsValidSESRegion("eu-west-3"))
	assert.True(t, domain.IsValidSESRegion("ap-southeast-1"))

	// The region is interpolated into the AWS endpoint host, so anything unknown is refused.
	for _, bad := range []string{"", "US-EAST-1", "evil.example.com", "../", "eu-west-3 "} {
		assert.False(t, domain.IsValidSESRegion(bad), "expected %q to be rejected", bad)
	}
}
