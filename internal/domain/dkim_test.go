package domain

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/pkg/dkim"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dkimTestSecret = "dkim-test-instance-secret"

func TestNormalizeDKIMDomain(t *testing.T) {
	valid := map[string]string{
		"acme.io":                       "acme.io",
		"  Acme.IO. ":                   "acme.io",
		"mail.acme.co.uk":               "mail.acme.co.uk",
		"bücher.de":                     "xn--bcher-kva.de",
		"a-b.example":                   "a-b.example",
		"xn--bcher-kva.de":              "xn--bcher-kva.de",
		"123.example":                   "123.example",
		strings.Repeat("a", 63) + ".io": strings.Repeat("a", 63) + ".io",
	}
	for raw, want := range valid {
		got, err := NormalizeDKIMDomain(raw)
		require.NoError(t, err, raw)
		assert.Equal(t, want, got, raw)
	}

	longest := strings.Repeat(strings.Repeat("a", 62)+".", 3) + strings.Repeat("a", 22) // 211
	require.Len(t, longest, MaxDKIMDomainLength)
	_, err := NormalizeDKIMDomain(longest)
	assert.NoError(t, err, "211 bytes is accepted")
	_, err = NormalizeDKIMDomain(longest + "a")
	assert.Error(t, err, "212 bytes is refused")

	invalid := []string{
		"", " ", "localhost", "io", "acme..io", "-acme.io", "acme-.io", "acme_io.com",
		"https://acme.io", "acme.io:25", "acme.io/path", "user@acme.io", "acme .io",
		"192.168.1.1", "[::1]", "acme.io\x00", strings.Repeat("a", 64) + ".io",
	}
	for _, raw := range invalid {
		_, err := NormalizeDKIMDomain(raw)
		assert.Error(t, err, "%q", raw)
	}
}

func TestEmailDomain(t *testing.T) {
	assert.Equal(t, "acme.io", EmailDomain("News@ACME.io"))
	assert.Equal(t, "xn--bcher-kva.de", EmailDomain("info@bücher.de"))
	assert.Equal(t, "", EmailDomain("no-at-sign"))
	assert.Equal(t, "", EmailDomain("x@localhost"))
}

func TestNewDKIMSelector(t *testing.T) {
	now := time.Date(2026, 11, 30, 23, 30, 0, 0, time.FixedZone("UTC+2", 2*3600))
	s, err := NewDKIMSelector(now, bytes.NewReader([]byte{0xa3, 0xf2, 0x9c, 0x1b, 0x04, 0xe8, 0x7d, 0x21}))
	require.NoError(t, err)
	assert.Equal(t, "notifuse202611a3f29c1b04e87d21", s, "the month is taken in UTC")
	assert.Len(t, s, 30)

	_, err = NewDKIMSelector(now, bytes.NewReader([]byte{1, 2}))
	assert.Error(t, err)
}

func TestDKIMKey_DNSRecordAndPublic(t *testing.T) {
	k := DKIMKey{Domain: "acme.io", Selector: "notifuse202611a3f29c1b04e87d21", PublicKey: "PUB", EncryptedPrivateKey: "SECRET"}
	assert.Equal(t, DKIMDNSRecord{
		Type:  "TXT",
		Name:  "notifuse202611a3f29c1b04e87d21._domainkey.acme.io",
		Value: "v=DKIM1; k=rsa; p=PUB",
	}, k.DNSRecord())
	assert.Empty(t, k.Public().EncryptedPrivateKey)
	assert.Equal(t, "SECRET", k.EncryptedPrivateKey, "Public does not touch the original")
}

func TestPublishedDKIMKeyFor(t *testing.T) {
	keys := []DKIMKey{
		{Domain: "acme.io", Status: DKIMKeyStatusPublished, EncryptedPrivateKey: "x"},
		{Domain: "pending.io", Status: DKIMKeyStatusPending, EncryptedPrivateKey: "x"},
		{Domain: "mismatch.io", Status: DKIMKeyStatusMismatch, EncryptedPrivateKey: "x"},
	}
	got := PublishedDKIMKeyFor(keys, "News@Acme.IO")
	require.NotNil(t, got)
	assert.Equal(t, "acme.io", got.Domain)

	assert.Nil(t, PublishedDKIMKeyFor(keys, "news@support.acme.io"), "a key does not sign a subdomain")
	assert.Nil(t, PublishedDKIMKeyFor(keys, "news@pending.io"))
	assert.Nil(t, PublishedDKIMKeyFor(keys, "news@mismatch.io"))
	assert.Nil(t, PublishedDKIMKeyFor(keys, "news@other.io"))
	assert.Nil(t, PublishedDKIMKeyFor(nil, "news@acme.io"))
}

func TestDKIMDomainUsers(t *testing.T) {
	integrations := Integrations{
		{ID: "smtp1", Name: "Relay", Type: IntegrationTypeEmail, EmailProvider: EmailProvider{
			Kind: EmailProviderKindSMTP, Senders: []EmailSender{{Email: "a@acme.io"}, {Email: "b@ACME.io"}},
		}},
		{ID: "smtp2", Name: "Sub", Type: IntegrationTypeEmail, EmailProvider: EmailProvider{
			Kind: EmailProviderKindSMTP, Senders: []EmailSender{{Email: "a@support.acme.io"}},
		}},
		{ID: "ses", Name: "SES", Type: IntegrationTypeEmail, EmailProvider: EmailProvider{
			Kind: EmailProviderKindSES, Senders: []EmailSender{{Email: "a@acme.io"}},
		}},
	}
	assert.Equal(t, []DKIMDomainUser{{IntegrationID: "smtp1", IntegrationName: "Relay"}}, DKIMDomainUsers(integrations, "acme.io"))
	assert.Equal(t, []DKIMDomainUser{}, DKIMDomainUsers(integrations, "other.io"))
	assert.True(t, EmailProviderKindSMTP.SignsWithNotifuseDKIM())
	assert.False(t, EmailProviderKindSES.SignsWithNotifuseDKIM())
}

func TestCheckDKIMKeys(t *testing.T) {
	privPEM, _, err := dkim.GenerateRSA2048()
	require.NoError(t, err)
	sealed, err := dkim.EncryptPrivateKey(privPEM, dkimTestSecret)
	require.NoError(t, err)
	notAKey, err := crypto.EncryptString("not a key", dkimTestSecret)
	require.NoError(t, err)

	settings := func(keys ...DKIMKey) []byte {
		b, err := json.Marshal(WorkspaceSettings{DKIMKeys: keys})
		require.NoError(t, err)
		return b
	}

	assert.NoError(t, CheckDKIMKeys(nil, dkimTestSecret))
	assert.NoError(t, CheckDKIMKeys(settings(), dkimTestSecret))
	assert.NoError(t, CheckDKIMKeys(settings(DKIMKey{Domain: "acme.io", EncryptedPrivateKey: sealed}), dkimTestSecret))
	assert.ErrorContains(t, CheckDKIMKeys(settings(DKIMKey{Domain: "acme.io", EncryptedPrivateKey: sealed}), "other-secret"), "acme.io")
	assert.Error(t, CheckDKIMKeys(settings(DKIMKey{Domain: "acme.io", EncryptedPrivateKey: "zz"}), dkimTestSecret))
	assert.Error(t, CheckDKIMKeys(settings(DKIMKey{Domain: "acme.io", EncryptedPrivateKey: notAKey}), dkimTestSecret))
}

func TestWorkspace_AfterLoadLeavesDKIMKeysSealed(t *testing.T) {
	encryptedSecret, err := crypto.EncryptString("workspace-secret", dkimTestSecret)
	require.NoError(t, err)
	w := Workspace{Settings: WorkspaceSettings{
		EncryptedSecretKey: encryptedSecret,
		DKIMKeys:           []DKIMKey{{Domain: "acme.io", EncryptedPrivateKey: "not-even-hex"}},
	}}
	require.NoError(t, w.AfterLoad(dkimTestSecret), "a broken DKIM key never fails a load")
	assert.Equal(t, "not-even-hex", w.Settings.DKIMKeys[0].EncryptedPrivateKey)

	settings, err := json.Marshal(w.Settings)
	require.NoError(t, err)
	integrations, err := json.Marshal(w.Integrations)
	require.NoError(t, err)
	assert.NoError(t, CheckStoredWorkspace(settings, integrations, dkimTestSecret),
		"saving other settings is not blocked by a broken DKIM key")
}

func TestWorkspaceRedact_RemovesDKIMKeys(t *testing.T) {
	w := &Workspace{Settings: WorkspaceSettings{DKIMKeys: []DKIMKey{{Domain: "acme.io", PublicKey: "PUB", EncryptedPrivateKey: "SECRET"}}}}
	w.Redact()
	assert.Nil(t, w.Settings.DKIMKeys)

	w = &Workspace{Settings: WorkspaceSettings{DKIMKeys: []DKIMKey{{Domain: "acme.io", EncryptedPrivateKey: "SECRET"}}}}
	w.RedactForPublic()
	raw, err := json.Marshal(w)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "dkim_keys")
}
