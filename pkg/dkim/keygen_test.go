package dkim

import (
	"crypto/x509"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateRSA2048(t *testing.T) {
	key, privPEM, pub := testKey(t)

	assert.True(t, strings.HasPrefix(privPEM, "-----BEGIN PRIVATE KEY-----"), "PKCS#8 PEM")
	assert.Equal(t, 2048, key.N.BitLen())
	assert.NotContains(t, pub, "\n")

	der, err := base64.StdEncoding.DecodeString(pub)
	require.NoError(t, err)
	_, err = x509.ParsePKIXPublicKey(der)
	require.NoError(t, err, "the public key is a SubjectPublicKeyInfo")

	parsed, err := ParsePublicKeyDER(pub)
	require.NoError(t, err)
	assert.Equal(t, 0, parsed.N.Cmp(key.N))
	assert.Equal(t, key.E, parsed.E)
}

func TestFormatDNSValue(t *testing.T) {
	assert.Equal(t, "v=DKIM1; k=rsa; p=ABC", FormatDNSValue("ABC"))
}

func TestParsePrivateKey(t *testing.T) {
	key, privPEM, _ := testKey(t)

	t.Run("PKCS#8", func(t *testing.T) {
		parsed, err := ParsePrivateKey(privPEM)
		require.NoError(t, err)
		assert.True(t, parsed.Equal(key))
	})
	t.Run("PKCS#1", func(t *testing.T) {
		parsed, err := ParsePrivateKey(pkcs1PEM(key))
		require.NoError(t, err)
		assert.True(t, parsed.Equal(key))
	})
	t.Run("a key under 2048 bits is refused", func(t *testing.T) {
		_, err := ParsePrivateKey(pkcs1PEM(smallRSAKey(t)))
		assert.ErrorContains(t, err, "fewer than 2048")
	})
	t.Run("not PEM", func(t *testing.T) {
		_, err := ParsePrivateKey("garbage")
		assert.Error(t, err)
	})
}

func TestParsePublicKeyDER(t *testing.T) {
	key, _, pub := testKey(t)

	t.Run("SPKI with whitespace", func(t *testing.T) {
		parsed, err := ParsePublicKeyDER(pub[:40] + " \t" + pub[40:])
		require.NoError(t, err)
		assert.Equal(t, 0, parsed.N.Cmp(key.N))
	})
	t.Run("PKCS#1", func(t *testing.T) {
		parsed, err := ParsePublicKeyDER(pkcs1PublicKeyB64(&key.PublicKey))
		require.NoError(t, err)
		assert.Equal(t, 0, parsed.N.Cmp(key.N))
	})
	t.Run("a key under 2048 bits is refused", func(t *testing.T) {
		_, err := ParsePublicKeyDER(pkcs1PublicKeyB64(&smallRSAKey(t).PublicKey))
		assert.ErrorContains(t, err, "fewer than 2048")
	})
	t.Run("not base64", func(t *testing.T) {
		_, err := ParsePublicKeyDER("%%%")
		assert.Error(t, err)
	})
}

func TestEncryptAndOpenPrivateKey(t *testing.T) {
	key, privPEM, _ := testKey(t)
	secret := "instance-secret"

	sealed, err := EncryptPrivateKey(privPEM, secret)
	require.NoError(t, err)
	assert.NotContains(t, sealed, "PRIVATE KEY")

	opened, err := OpenPrivateKey(sealed, secret)
	require.NoError(t, err)
	assert.True(t, opened.Equal(key))

	_, err = OpenPrivateKey(sealed, "another-secret")
	assert.Error(t, err)
}
