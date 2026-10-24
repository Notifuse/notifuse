package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func sesParserWithTransport(status int, body string, err error) *SESReplyParser {
	return NewSESReplyParser(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})})
}

func selfSignedCertPEM(t *testing.T, pub, priv interface{}) string {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "sns.amazonaws.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

const snsCertURL = "https://sns.us-east-1.amazonaws.com/SimpleNotificationService-cert.pem"

// signingKey is the SSRF boundary for SNS signatures: the cert URL must be a
// genuine SNS host, and every failure in fetch/parse must fail closed instead
// of falling back to trusting the message.
func TestSigningKey_FetchAndParse(t *testing.T) {
	ctx := context.Background()

	t.Run("non-SNS URLs are rejected before any fetch", func(t *testing.T) {
		for _, u := range []string{
			"http://sns.us-east-1.amazonaws.com/cert.pem",           // not https
			"https://sns.us-east-1.amazonaws.com.evil.com/cert.pem", // suffix attack
			"https://evil-sns.us-east-1.amazonaws.com/cert.pem",
			"https://169.254.169.254/latest/meta-data",
			"not a url %zz",
		} {
			p := sesParserWithTransport(200, "x", nil)
			_, err := p.signingKey(ctx, u)
			require.Error(t, err, u)
			assert.Contains(t, err.Error(), "signing cert:", u)
		}
	})

	t.Run("the .cn SNS host is accepted by validation", func(t *testing.T) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		p := sesParserWithTransport(200, selfSignedCertPEM(t, &key.PublicKey, key), nil)
		pub, err := p.signingKey(ctx, "https://sns.cn-north-1.amazonaws.com.cn/cert.pem")
		require.NoError(t, err)
		assert.True(t, key.PublicKey.Equal(pub))
	})

	t.Run("transport failure is wrapped, not swallowed", func(t *testing.T) {
		p := sesParserWithTransport(0, "", errors.New("connection refused"))
		_, err := p.signingKey(ctx, snsCertURL)
		require.ErrorContains(t, err, "fetch signing cert:")
		require.ErrorContains(t, err, "connection refused")
	})

	t.Run("non-200 status fails closed with the status", func(t *testing.T) {
		p := sesParserWithTransport(503, "oops", nil)
		_, err := p.signingKey(ctx, snsCertURL)
		require.EqualError(t, err, "signing cert fetch returned status 503")
	})

	t.Run("non-PEM body is rejected", func(t *testing.T) {
		p := sesParserWithTransport(200, "this is not pem", nil)
		_, err := p.signingKey(ctx, snsCertURL)
		require.EqualError(t, err, "signing cert is not valid PEM")
	})

	t.Run("PEM with garbage DER is rejected", func(t *testing.T) {
		garbage := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not a cert")}))
		p := sesParserWithTransport(200, garbage, nil)
		_, err := p.signingKey(ctx, snsCertURL)
		require.ErrorContains(t, err, "parse signing cert:")
	})

	t.Run("an ECDSA cert is rejected: SNS keys must be RSA", func(t *testing.T) {
		ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		p := sesParserWithTransport(200, selfSignedCertPEM(t, &ecKey.PublicKey, ecKey), nil)
		_, err = p.signingKey(ctx, snsCertURL)
		require.EqualError(t, err, "signing cert public key is not RSA")
	})

	t.Run("a fetched RSA key is cached: second call needs no network", func(t *testing.T) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		calls := 0
		p := NewSESReplyParser(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls > 1 {
				return nil, errors.New("network must not be touched on a cache hit")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(selfSignedCertPEM(t, &key.PublicKey, key))), Header: make(http.Header), Request: r}, nil
		})})
		pub1, err := p.signingKey(ctx, snsCertURL)
		require.NoError(t, err)
		pub2, err := p.signingKey(ctx, snsCertURL)
		require.NoError(t, err)
		assert.Same(t, pub1, pub2, "cache must return the identical key instance")
		assert.Equal(t, 1, calls)
	})
}

func TestSESReplyParser_SourceAndEnvelopeGuards(t *testing.T) {
	p := NewSESReplyParser(nil)
	assert.Equal(t, domain.WebhookSourceSES, p.Source())
	assert.Equal(t, domain.WebhookSource("ses"), p.Source())

	_, err := decodeSNSEnvelope(nil)
	require.EqualError(t, err, "ses inbound: empty request body")
	_, err = decodeSNSEnvelope(&domain.InboundRequest{})
	require.EqualError(t, err, "ses inbound: empty request body")
	_, err = decodeSNSEnvelope(&domain.InboundRequest{Body: []byte("{nope")})
	require.ErrorContains(t, err, "ses inbound: decode SNS envelope:")
}

// confirmSubscription must refuse non-SNS URLs (an attacker who could get us
// to GET an arbitrary URL after a forged signature check has an SSRF lever)
// and surface non-200s.
func TestConfirmSubscription_Guards(t *testing.T) {
	ctx := context.Background()
	p := sesParserWithTransport(200, "", nil)
	err := p.confirmSubscription(ctx, "https://169.254.169.254/confirm?token=x")
	require.ErrorContains(t, err, "URL is not a valid AWS SNS host")

	p = sesParserWithTransport(404, "", nil)
	err = p.confirmSubscription(ctx, "https://sns.us-east-1.amazonaws.com/?Action=ConfirmSubscription")
	require.EqualError(t, err, "subscribe URL returned status 404")

	p = sesParserWithTransport(200, "", nil)
	require.NoError(t, p.confirmSubscription(ctx, "https://sns.us-east-1.amazonaws.com/?Action=ConfirmSubscription"))
}

// sesHeaderMap keeps the FIRST value for a repeated header (later duplicates
// must not overwrite the signed original), lowercases names, and drops blanks.
func TestSESHeaderMapAndUnfold(t *testing.T) {
	m := sesHeaderMap([]domain.SESHeader{
		{Name: " Message-ID ", Value: "<first>"},
		{Name: "message-id", Value: "<second>"},
		{Name: "", Value: "ignored"},
		{Name: "X-Custom", Value: "v"},
	})
	assert.Equal(t, map[string]string{"message-id": "<first>", "x-custom": "v"}, m)

	lines := unfoldHeaderLines("Subject: hello\r\n\tworld again\r\nX-A: 1\n\nBody: not-a-header")
	assert.Equal(t, []string{"Subject: hello world again", "X-A: 1", "Body: not-a-header"}, lines)
}
