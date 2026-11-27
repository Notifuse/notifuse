package domain

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ResubscribeTokenTTL bounds the short-lived undo window after an unsubscribe so a contact
// who clicked by mistake can restore their membership (including on a private list) without
// turning a one-time undo into permanent access to private lists they left.
const ResubscribeTokenTTL = 30 * time.Minute

// resubscribeTokenKeyInfo separates the undo token signing key from email_hmac and tracking
// tokens derived from the same workspace secret key.
const resubscribeTokenKeyInfo = "notifuse-resubscribe-undo-v1"

var (
	// ErrResubscribeTokenInvalid reports a malformed or mismatched undo token.
	ErrResubscribeTokenInvalid = errors.New("invalid resubscribe token")
	// ErrResubscribeTokenExpired reports an undo token past its expiration window.
	ErrResubscribeTokenExpired = errors.New("resubscribe token expired")
	// ErrResubscribeNotAllowed reports that the membership can no longer be restored
	// (e.g. removed by the workspace, bounced, or complained).
	ErrResubscribeNotAllowed = errors.New("resubscribe not allowed")
)

func deriveResubscribeKey(workspaceSecretKey string) ([]byte, error) {
	if workspaceSecretKey == "" {
		return nil, errors.New("workspace secret key is empty")
	}
	return hkdf.Key(sha256.New, []byte(workspaceSecretKey), nil, resubscribeTokenKeyInfo, 32)
}

func computeResubscribeMAC(key []byte, workspaceID, email, listID string, exp int64) []byte {
	mac := hmac.New(sha256.New, key)
	// Length-prefixed fields prevent any delimiter ambiguity regardless of email or ID characters.
	for _, part := range []string{"resubscribe", workspaceID, email, listID, strconv.FormatInt(exp, 10)} {
		_, _ = mac.Write([]byte(strconv.Itoa(len(part))))
		_, _ = mac.Write([]byte{':'})
		_, _ = mac.Write([]byte(part))
		_, _ = mac.Write([]byte{'|'})
	}
	return mac.Sum(nil)
}

// MintResubscribeToken issues a stateless signed undo token valid for ResubscribeTokenTTL.
func MintResubscribeToken(workspaceSecretKey, workspaceID, email, listID string) (string, error) {
	return MintResubscribeTokenAt(workspaceSecretKey, workspaceID, email, listID, time.Now().UTC())
}

// MintResubscribeTokenAt issues a signed undo token expiring at now + ResubscribeTokenTTL.
func MintResubscribeTokenAt(workspaceSecretKey, workspaceID, email, listID string, now time.Time) (string, error) {
	if workspaceID == "" || email == "" || listID == "" {
		return "", errors.New("workspace_id, email and list_id are required")
	}
	key, err := deriveResubscribeKey(workspaceSecretKey)
	if err != nil {
		return "", err
	}
	exp := now.UTC().Add(ResubscribeTokenTTL).Unix()
	sig := computeResubscribeMAC(key, workspaceID, email, listID, exp)
	return fmt.Sprintf("%d.%s", exp, base64.RawURLEncoding.EncodeToString(sig)), nil
}

// VerifyResubscribeToken validates the signature, binding (workspace, email, list) and TTL of token.
func VerifyResubscribeToken(workspaceSecretKey, workspaceID, email, listID, token string) error {
	return VerifyResubscribeTokenAt(workspaceSecretKey, workspaceID, email, listID, token, time.Now().UTC())
}

// VerifyResubscribeTokenAt validates token against the given wall-clock time.
func VerifyResubscribeTokenAt(workspaceSecretKey, workspaceID, email, listID, token string, now time.Time) error {
	expStr, sigB64, ok := strings.Cut(token, ".")
	if !ok || expStr == "" || sigB64 == "" {
		return ErrResubscribeTokenInvalid
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || exp <= 0 {
		return ErrResubscribeTokenInvalid
	}
	providedSig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil || len(providedSig) != sha256.Size {
		return ErrResubscribeTokenInvalid
	}
	key, err := deriveResubscribeKey(workspaceSecretKey)
	if err != nil {
		return ErrResubscribeTokenInvalid
	}
	expectedSig := computeResubscribeMAC(key, workspaceID, email, listID, exp)
	if !hmac.Equal(expectedSig, providedSig) {
		return ErrResubscribeTokenInvalid
	}
	if now.UTC().Unix() > exp {
		return ErrResubscribeTokenExpired
	}
	return nil
}
