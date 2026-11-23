package dkim

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	testKeyOnce sync.Once
	testKeyPEM  string
	testKeyPub  string
)

// testKey generates one RSA 2048 key for the whole package: generating is slow.
func testKey(t *testing.T) (*rsa.PrivateKey, string, string) {
	t.Helper()
	testKeyOnce.Do(func() {
		var err error
		testKeyPEM, testKeyPub, err = GenerateRSA2048()
		require.NoError(t, err)
	})
	key, err := ParsePrivateKey(testKeyPEM)
	require.NoError(t, err)
	return key, testKeyPEM, testKeyPub
}

func pkcs1PublicKeyB64(key *rsa.PublicKey) string {
	return base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(key))
}

func smallRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)
	return key
}

func pkcs1PEM(key *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

// fakeResolver answers LookupTXT from a map and records the names asked.
type fakeResolver struct {
	records map[string][]string
	err     error
	asked   []string
}

func (f *fakeResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	f.asked = append(f.asked, name)
	if f.err != nil {
		return nil, f.err
	}
	records, ok := f.records[name]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
	return records, nil
}

func (f *fakeResolver) lookupTXT(name string) ([]string, error) {
	return f.LookupTXT(context.Background(), name)
}
