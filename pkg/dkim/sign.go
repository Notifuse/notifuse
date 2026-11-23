package dkim

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"errors"
	"fmt"
	"strings"

	msgdkim "github.com/emersion/go-msgauth/dkim"
)

// ErrSign wraps every failure to sign: a key that cannot be decrypted or parsed, or a
// signer error. It is a local error, never one of the provider's.
var ErrSign = errors.New("dkim sign error")

// signedHeaders are the header fields the SMTP profile signs, each occurrence the
// message carries. Date and Message-ID are left out on purpose: Amazon SES and other
// SMTP relays rewrite them, which would break a signature covering them (a deviation
// from the recommendation of RFC 6376 §5.4).
var signedHeaders = []string{
	"From", "To", "Cc", "Subject", "Reply-To", "MIME-Version", "Content-Type",
	"Content-Transfer-Encoding", "List-Unsubscribe", "List-Unsubscribe-Post",
	"In-Reply-To", "References",
}

// oversignedHeaders appear in h= once more than the message carries them (RFC 6376
// §5.4.2), so that adding one after signing, or injecting an absent one, breaks the
// signature.
var oversignedHeaders = map[string]bool{
	"from": true, "subject": true, "reply-to": true,
	"list-unsubscribe": true, "list-unsubscribe-post": true,
}

// SignParams names the key that signs a message.
type SignParams struct {
	Domain   string
	Selector string
	Key      *rsa.PrivateKey
}

// Sign returns message with a DKIM-Signature field prepended: rsa-sha256,
// relaxed/relaxed, the whole body (never l=). message is the raw message, header and
// body separated by an empty line, with CRLF line endings.
func Sign(message []byte, p SignParams) ([]byte, error) {
	if p.Key == nil {
		return nil, fmt.Errorf("%w: no key", ErrSign)
	}
	var out bytes.Buffer
	err := msgdkim.Sign(&out, bytes.NewReader(message), &msgdkim.SignOptions{
		Domain:                 p.Domain,
		Selector:               p.Selector,
		Signer:                 p.Key,
		Hash:                   crypto.SHA256,
		HeaderCanonicalization: msgdkim.CanonicalizationRelaxed,
		BodyCanonicalization:   msgdkim.CanonicalizationRelaxed,
		HeaderKeys:             HeaderKeys(message),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSign, err)
	}
	return out.Bytes(), nil
}

// HeaderKeys is the h= list for message: each signed header field as many times as the
// message carries it, plus once for the oversigned ones.
func HeaderKeys(message []byte) []string {
	counts := headerCounts(message)
	var keys []string
	for _, name := range signedHeaders {
		n := counts[strings.ToLower(name)]
		if oversignedHeaders[strings.ToLower(name)] {
			n++
		}
		for i := 0; i < n; i++ {
			keys = append(keys, name)
		}
	}
	return keys
}

// headerCounts counts the header fields of message by lower-case name. Folded
// continuation lines (starting with a space or a tab) belong to the field above them.
func headerCounts(message []byte) map[string]int {
	counts := map[string]int{}
	for _, line := range strings.Split(string(message), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			break
		}
		if line[0] == ' ' || line[0] == '\t' {
			continue
		}
		name, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		counts[strings.ToLower(strings.TrimSpace(name))]++
	}
	return counts
}
