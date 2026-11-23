// Package dkim generates the RSA keys Notifuse signs workspace emails with, checks the
// TXT record a domain publishes for them, and signs a raw MIME message (RFC 6376).
//
// It knows nothing about workspaces: the caller decides which key signs which message,
// and only ever hands over a key whose record was found published.
package dkim

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"github.com/Notifuse/notifuse/pkg/crypto"
)

// KeyBits is the size of the keys GenerateRSA2048 makes, and the smallest size a key
// read back is accepted with.
const KeyBits = 2048

// GenerateRSA2048 returns a new RSA key: the private half as a PKCS#8 PEM block, and the
// public half as the base64 of its DER SubjectPublicKeyInfo, with no line breaks, the
// form that goes after p= in the TXT record (what opendkim-genkey publishes).
func GenerateRSA2048() (privateKeyPEM string, publicKeyBase64 string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, KeyBits)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate the RSA key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("failed to encode the private key: %w", err)
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to encode the public key: %w", err)
	}
	block := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return string(block), base64.StdEncoding.EncodeToString(pub), nil
}

// FormatDNSValue is the TXT record value that publishes a public key.
func FormatDNSValue(publicKeyBase64 string) string {
	return "v=DKIM1; k=rsa; p=" + publicKeyBase64
}

// ParsePrivateKey reads a PEM private key, PKCS#8 or PKCS#1, and accepts only an RSA
// key of at least KeyBits.
func ParsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	var key *rsa.PrivateKey
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("the private key is not an RSA key")
		}
		key = rsaKey
	} else if rsaKey, err1 := x509.ParsePKCS1PrivateKey(block.Bytes); err1 == nil {
		key = rsaKey
	} else {
		return nil, fmt.Errorf("failed to parse the private key: %w", err)
	}
	if key.N.BitLen() < KeyBits {
		return nil, fmt.Errorf("the RSA key has %d bits, fewer than %d", key.N.BitLen(), KeyBits)
	}
	return key, nil
}

// ParsePublicKeyDER reads the base64 of a DER RSA public key, SubjectPublicKeyInfo or
// PKCS#1 (RFC 6376 erratum 3017: both are deployed), ignoring any whitespace in it, and
// accepts only a key of at least KeyBits.
func ParsePublicKeyDER(b64 string) (*rsa.PublicKey, error) {
	der, err := base64.StdEncoding.DecodeString(stripWhitespace(b64))
	if err != nil {
		return nil, fmt.Errorf("the public key is not valid base64: %w", err)
	}
	var key *rsa.PublicKey
	if parsed, err := x509.ParsePKIXPublicKey(der); err == nil {
		rsaKey, ok := parsed.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("the public key is not an RSA key")
		}
		key = rsaKey
	} else if rsaKey, err1 := x509.ParsePKCS1PublicKey(der); err1 == nil {
		key = rsaKey
	} else {
		return nil, fmt.Errorf("failed to parse the public key: %w", err)
	}
	if key.N.BitLen() < KeyBits {
		return nil, fmt.Errorf("the RSA key has %d bits, fewer than %d", key.N.BitLen(), KeyBits)
	}
	return key, nil
}

// EncryptPrivateKey seals a PEM private key with the instance secret key, the way the
// other workspace credentials are stored.
func EncryptPrivateKey(privateKeyPEM, globalSecretKey string) (string, error) {
	return crypto.EncryptString(privateKeyPEM, globalSecretKey)
}

// OpenPrivateKey decrypts a stored private key with the instance secret key and parses
// it.
func OpenPrivateKey(encrypted, globalSecretKey string) (*rsa.PrivateKey, error) {
	pemStr, err := crypto.DecryptFromHexString(encrypted, globalSecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt the private key: %w", err)
	}
	return ParsePrivateKey(pemStr)
}

func stripWhitespace(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		}
		return r
	}, s)
}
