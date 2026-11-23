package domain

import (
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/pkg/dkim"
	"golang.org/x/net/idna"
)

// DKIM keys of a workspace. Notifuse signs the emails a workspace sends through an SMTP
// integration with the key of the sender's domain, once the domain publishes the key
// (status published). The keys live in the workspace settings, under dkim_keys, and
// only WorkspaceRepository.UpdateDKIMKeys writes them: Update() keeps the stored ones.
// The private key stays encrypted with the instance secret key until a send needs it:
// AfterLoad never opens it.

// DKIMKeyStatus says whether a key signs.
type DKIMKeyStatus string

const (
	// DKIMKeyStatusPending: no record found yet (or any more), the key does not sign.
	DKIMKeyStatusPending DKIMKeyStatus = "pending"
	// DKIMKeyStatusPublished: the record carries this key, the key signs.
	DKIMKeyStatusPublished DKIMKeyStatus = "published"
	// DKIMKeyStatusMismatch: a record is there but invalid, revoked or for another key;
	// the key does not sign.
	DKIMKeyStatusMismatch DKIMKeyStatus = "mismatch"
)

// DKIMCheckResult is what one lookup of a key's record found.
type DKIMCheckResult string

const (
	DKIMCheckResultOK       DKIMCheckResult = DKIMCheckResult(dkim.ResultOK)
	DKIMCheckResultNotFound DKIMCheckResult = DKIMCheckResult(dkim.ResultNotFound)
	DKIMCheckResultMismatch DKIMCheckResult = DKIMCheckResult(dkim.ResultMismatch)
	DKIMCheckResultDNSError DKIMCheckResult = DKIMCheckResult(dkim.ResultDNSError)
)

const (
	// MaxDKIMKeysPerWorkspace caps the domains of one workspace.
	MaxDKIMKeysPerWorkspace = 100
	// MaxDKIMDomainLength keeps <selector>._domainkey.<domain> within the 253 bytes of
	// a DNS name: 253 - 30 (selector) - 12 (._domainkey.) = 211.
	MaxDKIMDomainLength = 211
	// dkimSelectorPrefix starts every selector Notifuse makes.
	dkimSelectorPrefix = "notifuse"
	// maxDKIMCheckErrorLength bounds the stored reason of a failed check.
	maxDKIMCheckErrorLength = 255
)

// DKIMKey is the key of one domain of a workspace.
type DKIMKey struct {
	ID                  string        `json:"id"`       // changes with every generated key
	Revision            int64         `json:"revision"` // incremented by every applied check
	Domain              string        `json:"domain"`   // normalised, ASCII
	Selector            string        `json:"selector"`
	PublicKey           string        `json:"public_key"` // base64 DER SubjectPublicKeyInfo
	EncryptedPrivateKey string        `json:"encrypted_private_key,omitempty"`
	Status              DKIMKeyStatus `json:"status"`
	CreatedAt           time.Time     `json:"created_at"`
	VerifiedAt          *time.Time    `json:"verified_at,omitempty"`
	// DowngradedAt and DowngradeReason say when and why a published key stopped signing.
	DowngradedAt         *time.Time `json:"downgraded_at,omitempty"`
	DowngradeReason      string     `json:"downgrade_reason,omitempty"`
	LastCheckedAt        *time.Time `json:"last_checked_at,omitempty"`
	LastCheckError       string     `json:"last_check_error,omitempty"`
	ConsecutiveFailures  int        `json:"consecutive_failures,omitempty"`
	LastCountedFailureAt *time.Time `json:"last_counted_failure_at,omitempty"`
}

// DKIMDNSRecord is the record a domain must publish for its key.
type DKIMDNSRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// DNSRecord is the TXT record that publishes the key.
func (k DKIMKey) DNSRecord() DKIMDNSRecord {
	return DKIMDNSRecord{
		Type:  "TXT",
		Name:  dkim.RecordName(k.Selector, k.Domain),
		Value: dkim.FormatDNSValue(k.PublicKey),
	}
}

// Public returns the key without its private half.
func (k DKIMKey) Public() DKIMKey {
	k.EncryptedPrivateKey = ""
	return k
}

// DKIMTransition is a change of a key's status a check applied. The downgrade of a
// published key is logged at warn level; the email to the owners comes in v46.
type DKIMTransition struct {
	WorkspaceID string
	Domain      string
	Selector    string
	OldStatus   DKIMKeyStatus
	NewStatus   DKIMKeyStatus
	Reason      string
	OccurredAt  time.Time
}

// DKIMSigningKey is the key a send signs with, resolved by EmailService.
type DKIMSigningKey struct {
	Domain   string
	Selector string
	Key      *rsa.PrivateKey
}

// WorkspaceDKIMMetadata is the keys of one workspace, without their private half.
type WorkspaceDKIMMetadata struct {
	WorkspaceID string
	Keys        []DKIMKey
}

// DKIMKeysMutation is what a DKIMKeysMutator decided.
type DKIMKeysMutation struct {
	// Keys replace the stored keys when Changed.
	Keys    []DKIMKey
	Changed bool
	// TouchUpdatedAt moves the workspace's updated_at: a key created, regenerated or
	// deleted, or a status change. Not a check that only moved last_checked_at.
	TouchUpdatedAt bool
}

// DKIMKeysMutator decides, under the workspace row lock, how the stored keys change.
// integrations are as stored (credentials encrypted), for the senders of each.
type DKIMKeysMutator func(keys []DKIMKey, integrations Integrations) (DKIMKeysMutation, error)

// DKIMDomainUser is an integration with a sender on a domain.
type DKIMDomainUser struct {
	IntegrationID   string `json:"integration_id"`
	IntegrationName string `json:"integration_name"`
}

var dkimLabelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// NormalizeDKIMDomain returns the ASCII, lower-case form of a domain a key can be
// created for, or an error saying why it cannot.
func NormalizeDKIMDomain(raw string) (string, error) {
	d := strings.TrimSpace(raw)
	d = strings.TrimSuffix(d, ".")
	if d == "" {
		return "", errors.New("domain is required")
	}
	if strings.ContainsAny(d, " \t\r\n/:@?#") || strings.ContainsFunc(d, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", fmt.Errorf("%q is not a domain name", raw)
	}
	if net.ParseIP(strings.Trim(d, "[]")) != nil {
		return "", fmt.Errorf("%q is an IP address, not a domain name", raw)
	}
	ascii, err := idna.Lookup.ToASCII(d)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid domain name: %v", raw, err)
	}
	ascii = strings.ToLower(ascii)
	if len(ascii) > MaxDKIMDomainLength {
		return "", fmt.Errorf("domain is longer than %d characters", MaxDKIMDomainLength)
	}
	labels := strings.Split(ascii, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("%q is not a full domain name", raw)
	}
	for _, label := range labels {
		if !dkimLabelPattern.MatchString(label) {
			return "", fmt.Errorf("%q is not a valid domain name", raw)
		}
	}
	return ascii, nil
}

// EmailDomain is the normalised domain of an email address, or "" when it has none.
func EmailDomain(address string) string {
	at := strings.LastIndex(address, "@")
	if at < 0 {
		return ""
	}
	d, err := NormalizeDKIMDomain(strings.TrimSuffix(address[at+1:], ">"))
	if err != nil {
		return ""
	}
	return d
}

// NewDKIMSelector returns notifuse + YYYYMM (UTC) + 16 random hex characters: 30
// characters, so that two keys never share a record name, whatever the workspace,
// installation or month.
func NewDKIMSelector(now time.Time, random io.Reader) (string, error) {
	b := make([]byte, 8)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", fmt.Errorf("failed to draw the selector: %w", err)
	}
	return dkimSelectorPrefix + now.UTC().Format("200601") + hex.EncodeToString(b), nil
}

// FindDKIMKey returns the index of the key of domain (normalised), or -1.
func FindDKIMKey(keys []DKIMKey, domain string) int {
	for i := range keys {
		if keys[i].Domain == domain {
			return i
		}
	}
	return -1
}

// PublishedDKIMKeyFor returns the published key of the exact domain of fromAddress: a
// key on acme.io does not sign support.acme.io.
func PublishedDKIMKeyFor(keys []DKIMKey, fromAddress string) *DKIMKey {
	d := EmailDomain(fromAddress)
	if d == "" {
		return nil
	}
	i := FindDKIMKey(keys, d)
	if i < 0 || keys[i].Status != DKIMKeyStatusPublished || keys[i].EncryptedPrivateKey == "" {
		return nil
	}
	k := keys[i]
	return &k
}

// SignsWithNotifuseDKIM says whether Notifuse signs the emails of this provider itself.
// The API providers sign on their side.
func (k EmailProviderKind) SignsWithNotifuseDKIM() bool {
	return k == EmailProviderKindSMTP
}

// DKIMDomainUsers lists the integrations Notifuse signs for that have a sender on the
// exact domain.
func DKIMDomainUsers(integrations Integrations, domain string) []DKIMDomainUser {
	users := []DKIMDomainUser{}
	for _, integration := range integrations {
		if integration.Type != IntegrationTypeEmail || !integration.EmailProvider.Kind.SignsWithNotifuseDKIM() {
			continue
		}
		for _, sender := range integration.EmailProvider.Senders {
			if EmailDomain(sender.Email) == domain {
				users = append(users, DKIMDomainUser{IntegrationID: integration.ID, IntegrationName: integration.Name})
				break
			}
		}
	}
	return users
}

// CheckDKIMKeys checks that every private key in the stored settings opens with
// globalSecretKey and is a valid RSA key. RotateSecretKey and UpdateDKIMKeys call it;
// Update() does not, so that a broken DKIM key never blocks saving other settings.
func CheckDKIMKeys(settingsJSON []byte, globalSecretKey string) error {
	if len(settingsJSON) == 0 {
		return nil
	}
	var settings struct {
		DKIMKeys []DKIMKey `json:"dkim_keys"`
	}
	if err := json.Unmarshal(settingsJSON, &settings); err != nil {
		return fmt.Errorf("failed to read the DKIM keys: %w", err)
	}
	for _, k := range settings.DKIMKeys {
		if k.EncryptedPrivateKey == "" {
			continue
		}
		if _, err := dkim.OpenPrivateKey(k.EncryptedPrivateKey, globalSecretKey); err != nil {
			return fmt.Errorf("DKIM key of %s: %w", k.Domain, err)
		}
	}
	return nil
}

func truncateDKIMCheckError(s string) string {
	if len(s) <= maxDKIMCheckErrorLength {
		return s
	}
	r := []rune(s)
	for len(string(r)) > maxDKIMCheckErrorLength {
		r = r[:len(r)-1]
	}
	return string(r)
}
