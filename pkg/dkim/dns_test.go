package dkim

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifyDNS(t *testing.T) {
	key, _, pub := testKey(t)
	const selector, domain = "notifuse202611a3f29c1b04e87d21", "acme.io"
	name := RecordName(selector, domain) + "."

	cases := []struct {
		name    string
		records []string
		err     error
		want    Result
	}{
		{"published SPKI", []string{FormatDNSValue(pub)}, nil, ResultOK},
		{"published PKCS#1", []string{FormatDNSValue(pkcs1PublicKeyB64(&key.PublicKey))}, nil, ResultOK},
		{"no v, k or trailing separator issues", []string{"p=" + pub + ";"}, nil, ResultOK},
		{"whitespace inside p", []string{"v=DKIM1; k=rsa; p=" + pub[:30] + " " + pub[30:]}, nil, ResultOK},
		{"h and s allow it", []string{"v=DKIM1; h=sha1:sha256; s=email; p=" + pub}, nil, ResultOK},
		{"s=*", []string{"v=DKIM1; s=*; p=" + pub}, nil, ResultOK},
		{"two TXT records", []string{FormatDNSValue(pub), FormatDNSValue(pub)}, nil, ResultMismatch},
		{"tag twice", []string{"v=DKIM1; p=" + pub + "; p=" + pub}, nil, ResultMismatch},
		{"v not first", []string{"k=rsa; v=DKIM1; p=" + pub}, nil, ResultMismatch},
		{"v=DKIM2", []string{"v=DKIM2; p=" + pub}, nil, ResultMismatch},
		{"k=ed25519", []string{"v=DKIM1; k=ed25519; p=" + pub}, nil, ResultMismatch},
		{"h=sha1 only", []string{"v=DKIM1; h=sha1; p=" + pub}, nil, ResultMismatch},
		{"s=foo", []string{"v=DKIM1; s=foo; p=" + pub}, nil, ResultMismatch},
		{"revoked, empty p", []string{"v=DKIM1; k=rsa; p="}, nil, ResultMismatch},
		{"no p", []string{"v=DKIM1; k=rsa"}, nil, ResultMismatch},
		{"malformed tag", []string{"v=DKIM1; garbage; p=" + pub}, nil, ResultMismatch},
		{"another key", []string{FormatDNSValue(regenerated(t))}, nil, ResultMismatch},
		{"no records", []string{}, nil, ResultNotFound},
		{"NXDOMAIN", nil, &net.DNSError{Err: "no such host", IsNotFound: true}, ResultNotFound},
		{"timeout", nil, &net.DNSError{Err: "i/o timeout", IsTimeout: true}, ResultDNSError},
		{"SERVFAIL", nil, &net.DNSError{Err: "server misbehaving"}, ResultDNSError},
		{"other error", nil, errors.New("boom"), ResultDNSError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeResolver{records: map[string][]string{}, err: tc.err}
			if tc.records != nil {
				r.records[name] = tc.records
			}
			got, detail := VerifyDNS(context.Background(), r, selector, domain, pub)
			assert.Equal(t, tc.want, got, detail)
			if got != ResultOK {
				assert.NotEmpty(t, detail)
			}
			assert.Equal(t, []string{name}, r.asked, "the name is queried fully qualified")
		})
	}
}

func regenerated(t *testing.T) string {
	t.Helper()
	_, pub, err := GenerateRSA2048()
	if err != nil {
		t.Fatal(err)
	}
	return pub
}
