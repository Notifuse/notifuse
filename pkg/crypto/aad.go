package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// ErrEmptyAAD is returned when a value is sealed or opened without associated data.
// The AAD variants exist to bind a ciphertext to its owner; an empty one binds nothing.
var ErrEmptyAAD = errors.New("associated data must not be empty")

// EncryptStringAAD seals str like EncryptString (AES-256-GCM, key derived from passphrase
// by SHA-256, hex of nonce followed by ciphertext), and binds it to aad: the value only
// opens with the same aad. Use it for a secret that belongs to one row, with the row's
// owner as aad, so a ciphertext copied onto another row does not open there.
func EncryptStringAAD(str, passphrase, aad string) (string, error) {
	if aad == "" {
		return "", ErrEmptyAAD
	}
	gcm, err := newGCM(passphrase)
	if err != nil {
		return "", fmt.Errorf("EncryptStringAAD: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("EncryptStringAAD reader error: %w", err)
	}
	return hex.EncodeToString(gcm.Seal(nonce, nonce, []byte(str), []byte(aad))), nil
}

// DecryptFromHexStringAAD opens a value EncryptStringAAD sealed with the same passphrase
// and aad. A different aad fails like a different key.
func DecryptFromHexStringAAD(str, passphrase, aad string) (string, error) {
	if aad == "" {
		return "", ErrEmptyAAD
	}
	if str == "" {
		return "", errors.New("DecryptFromHexStringAAD empty string")
	}
	data, err := hex.DecodeString(str)
	if err != nil {
		return "", fmt.Errorf("DecryptFromHexStringAAD decode error: %w", err)
	}
	gcm, err := newGCM(passphrase)
	if err != nil {
		return "", fmt.Errorf("DecryptFromHexStringAAD: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("DecryptFromHexStringAAD data too short: got %d bytes, need at least %d", len(data), gcm.NonceSize())
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(aad))
	if err != nil {
		return "", fmt.Errorf("DecryptFromHexStringAAD open error: %w", err)
	}
	return string(plaintext), nil
}

// DecryptWithKeysAAD opens a value EncryptStringAAD sealed, with the first of keys that
// opens it, and says which one, as DecryptWithKeys does for EncryptString.
func DecryptWithKeysAAD(value string, keys []string, aad string) (string, int, error) {
	if aad == "" {
		return "", -1, ErrEmptyAAD
	}
	for i, key := range keys {
		if plaintext, err := DecryptFromHexStringAAD(value, key, aad); err == nil {
			return plaintext, i, nil
		}
	}
	return "", -1, ErrUndecryptable
}

// ReencryptValueAAD makes a value EncryptStringAAD sealed open with current, keeping its
// aad, as ReencryptValue does for EncryptString. A value current already opens comes back
// unchanged.
func ReencryptValueAAD(value, current string, previous []string, aad string) (string, bool, error) {
	plaintext, index, err := DecryptWithKeysAAD(value, append([]string{current}, previous...), aad)
	if err != nil {
		return "", false, err
	}
	if index == 0 {
		return value, false, nil
	}
	reencrypted, err := EncryptStringAAD(plaintext, current, aad)
	if err != nil {
		return "", false, err
	}
	return reencrypted, true, nil
}

func newGCM(passphrase string) (cipher.AEAD, error) {
	block, err := aes.NewCipher(Sha256Hash(passphrase))
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return gcm, nil
}
