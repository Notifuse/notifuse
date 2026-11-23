package crypto

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	aadUser  = "notifuse:totp:6f1c2a7e-0b5d-4c1e-9a3f-2d8e7b6c5a41"
	aadOther = "notifuse:totp:0a9b8c7d-6e5f-4a3b-2c1d-0e9f8a7b6c5d"
)

func TestEncryptStringAADRoundTrip(t *testing.T) {
	sealed, err := EncryptStringAAD("JBSWY3DPEHPK3PXP", "current-key", aadUser)
	require.NoError(t, err)

	opened, err := DecryptFromHexStringAAD(sealed, "current-key", aadUser)
	require.NoError(t, err)
	assert.Equal(t, "JBSWY3DPEHPK3PXP", opened)

	again, err := EncryptStringAAD("JBSWY3DPEHPK3PXP", "current-key", aadUser)
	require.NoError(t, err)
	assert.NotEqual(t, sealed, again, "a fresh nonce each time")
}

func TestDecryptFromHexStringAADRefusesAnotherOwner(t *testing.T) {
	sealed, err := EncryptStringAAD("secret", "current-key", aadUser)
	require.NoError(t, err)

	_, err = DecryptFromHexStringAAD(sealed, "current-key", aadOther)
	assert.Error(t, err, "a ciphertext copied onto another user's row must not open")

	_, err = DecryptFromHexStringAAD(sealed, "other-key", aadUser)
	assert.Error(t, err)
}

func TestAADMustNotBeEmpty(t *testing.T) {
	_, err := EncryptStringAAD("secret", "current-key", "")
	assert.True(t, errors.Is(err, ErrEmptyAAD))

	sealed, err := EncryptStringAAD("secret", "current-key", aadUser)
	require.NoError(t, err)
	_, err = DecryptFromHexStringAAD(sealed, "current-key", "")
	assert.True(t, errors.Is(err, ErrEmptyAAD))

	_, _, err = DecryptWithKeysAAD(sealed, []string{"current-key"}, "")
	assert.True(t, errors.Is(err, ErrEmptyAAD))
}

func TestAADAndPlainEncryptionDoNotOpenEachOther(t *testing.T) {
	plain, err := EncryptString("secret", "current-key")
	require.NoError(t, err)
	_, err = DecryptFromHexStringAAD(plain, "current-key", aadUser)
	assert.Error(t, err, "a value sealed without associated data does not open with some")

	bound, err := EncryptStringAAD("secret", "current-key", aadUser)
	require.NoError(t, err)
	_, err = DecryptFromHexString(bound, "current-key")
	assert.Error(t, err, "a value sealed with associated data does not open without it")
}

func TestDecryptFromHexStringAADRefusesMalformedInput(t *testing.T) {
	for _, value := range []string{"", "zz", "0a0b"} {
		_, err := DecryptFromHexStringAAD(value, "current-key", aadUser)
		assert.Error(t, err, "value %q", value)
	}
}

func TestDecryptWithKeysAADSaysWhichKeyOpened(t *testing.T) {
	sealed, err := EncryptStringAAD("secret", "old-key", aadUser)
	require.NoError(t, err)

	opened, index, err := DecryptWithKeysAAD(sealed, []string{"new-key", "old-key"}, aadUser)
	require.NoError(t, err)
	assert.Equal(t, "secret", opened)
	assert.Equal(t, 1, index)

	_, index, err = DecryptWithKeysAAD(sealed, []string{"new-key", "another-key"}, aadUser)
	assert.True(t, errors.Is(err, ErrUndecryptable))
	assert.Equal(t, -1, index)
}

func TestReencryptValueAAD(t *testing.T) {
	sealed, err := EncryptStringAAD("secret", "old-key", aadUser)
	require.NoError(t, err)

	reencrypted, changed, err := ReencryptValueAAD(sealed, "new-key", []string{"old-key"}, aadUser)
	require.NoError(t, err)
	assert.True(t, changed)
	opened, err := DecryptFromHexStringAAD(reencrypted, "new-key", aadUser)
	require.NoError(t, err)
	assert.Equal(t, "secret", opened)

	same, changed, err := ReencryptValueAAD(reencrypted, "new-key", []string{"old-key"}, aadUser)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, reencrypted, same)

	_, _, err = ReencryptValueAAD(sealed, "new-key", nil, aadUser)
	assert.True(t, errors.Is(err, ErrUndecryptable), "no key opens it")

	_, _, err = ReencryptValueAAD(sealed, "new-key", []string{"old-key"}, aadOther)
	assert.True(t, errors.Is(err, ErrUndecryptable), "the aad is part of what must match")
}
