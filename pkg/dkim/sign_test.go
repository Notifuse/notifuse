package dkim

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"

	msgdkim "github.com/emersion/go-msgauth/dkim"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mail "github.com/wneessen/go-mail"
)

const (
	testDomain   = "acme.io"
	testSelector = "notifuse202611a3f29c1b04e87d21"
)

type composeOptions struct {
	replyTo     bool
	unsubscribe bool
	attachment  bool
}

// compose builds a message the way SMTPService.SendEmail does.
func compose(t *testing.T, o composeOptions) []byte {
	t.Helper()
	msg := mail.NewMsg(mail.WithNoDefaultUserAgent())
	require.NoError(t, msg.FromFormat("Acme", "news@acme.io"))
	require.NoError(t, msg.To("jane@example.com"))
	if o.replyTo {
		require.NoError(t, msg.ReplyTo("support@acme.io"))
	}
	msg.SetGenHeader("X-Message-ID", "msg-1")
	if o.unsubscribe {
		msg.SetGenHeader("List-Unsubscribe", "<https://acme.io/u/1>")
		msg.SetGenHeader("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	}
	msg.Subject("Événement d'été à Zürich")
	msg.SetBodyString(mail.TypeTextPlain, "Bonjour Zoé,\n.leading dot line\nÀ bientôt")
	msg.AddAlternativeString(mail.TypeTextHTML, "<p>Bonjour Zoé, à bientôt.</p>\n.<b>dot</b>")
	if o.attachment {
		require.NoError(t, msg.AttachReader("report.bin", bytes.NewReader([]byte{0, 1, 2, 0xff, 0xfe})))
	}
	var buf bytes.Buffer
	_, err := msg.WriteTo(&buf)
	require.NoError(t, err)
	return buf.Bytes()
}

func sign(t *testing.T, message []byte) []byte {
	t.Helper()
	key, _, _ := testKey(t)
	signed, err := Sign(message, SignParams{Domain: testDomain, Selector: testSelector, Key: key})
	require.NoError(t, err)
	return signed
}

// verify checks the single signature of message against the test key.
func verify(t *testing.T, message []byte) error {
	t.Helper()
	_, _, pub := testKey(t)
	r := &fakeResolver{records: map[string][]string{
		RecordName(testSelector, testDomain): {FormatDNSValue(pub)},
	}}
	verifications, err := msgdkim.VerifyWithOptions(bytes.NewReader(message), &msgdkim.VerifyOptions{LookupTXT: r.lookupTXT})
	require.NoError(t, err)
	require.Len(t, verifications, 1)
	return verifications[0].Err
}

func signatureTag(t *testing.T, signed []byte, tag string) (string, bool) {
	t.Helper()
	header := string(signed[:bytes.Index(signed, []byte("\r\n\r\n"))])
	start := strings.Index(header, "DKIM-Signature:")
	require.GreaterOrEqual(t, start, 0)
	field := header[start:]
	// The field ends at the first line that does not start with whitespace.
	if end := regexp.MustCompile(`\r\n[^ \t]`).FindStringIndex(field); end != nil {
		field = field[:end[0]]
	}
	field = regexp.MustCompile(`\s+`).ReplaceAllString(field, "")
	for _, part := range strings.Split(strings.TrimPrefix(field, "DKIM-Signature:"), ";") {
		if name, value, ok := strings.Cut(part, "="); ok && name == tag {
			return value, true
		}
	}
	return "", false
}

func TestSign_Verifies(t *testing.T) {
	message := compose(t, composeOptions{replyTo: true, unsubscribe: true, attachment: true})
	signed := sign(t, message)

	assert.NoError(t, verify(t, signed))
	assert.True(t, bytes.HasSuffix(signed, message), "the message follows the signature unchanged")

	a, _ := signatureTag(t, signed, "a")
	c, _ := signatureTag(t, signed, "c")
	d, _ := signatureTag(t, signed, "d")
	s, _ := signatureTag(t, signed, "s")
	assert.Equal(t, "rsa-sha256", a)
	assert.Equal(t, "relaxed/relaxed", c)
	assert.Equal(t, testDomain, d)
	assert.Equal(t, testSelector, s)
	_, hasL := signatureTag(t, signed, "l")
	assert.False(t, hasL, "the whole body is signed")
}

func TestSign_BodyIs7Bit(t *testing.T) {
	message := compose(t, composeOptions{attachment: true})
	for i, b := range message {
		require.Less(t, b, byte(0x80), "byte %d is not 7-bit", i)
	}
	body := string(message)
	assert.Contains(t, body, "Content-Transfer-Encoding: quoted-printable")
	assert.Contains(t, body, "Content-Transfer-Encoding: base64")
}

func TestSign_HeaderList(t *testing.T) {
	signed := sign(t, compose(t, composeOptions{replyTo: true, unsubscribe: true}))
	h, ok := signatureTag(t, signed, "h")
	require.True(t, ok)
	keys := strings.Split(strings.ToLower(h), ":")

	count := func(name string) int {
		n := 0
		for _, k := range keys {
			if k == name {
				n++
			}
		}
		return n
	}
	assert.Zero(t, count("date"), "Date is not signed")
	assert.Zero(t, count("message-id"), "Message-ID is not signed")
	for _, name := range []string{"from", "subject", "reply-to", "list-unsubscribe", "list-unsubscribe-post"} {
		assert.Equal(t, 2, count(name), "%s is present once and oversigned", name)
	}
	for _, name := range []string{"to", "mime-version", "content-type"} {
		assert.Equal(t, 1, count(name), name)
	}
}

func TestSign_DateAndMessageIDCanBeRewritten(t *testing.T) {
	signed := sign(t, compose(t, composeOptions{}))
	rewritten := regexp.MustCompile(`(?m)^Date: .*\r$`).ReplaceAll(signed, []byte("Date: Mon, 1 Jan 2024 00:00:00 +0000\r"))
	rewritten = regexp.MustCompile(`(?m)^Message-ID: .*\r$`).ReplaceAll(rewritten, []byte("Message-ID: <rewritten@relay.example>\r"))
	require.NotEqual(t, signed, rewritten)
	assert.NoError(t, verify(t, rewritten))
}

func TestSign_AddedHeadersBreakTheSignature(t *testing.T) {
	cases := map[string]struct {
		options composeOptions
		header  string
	}{
		"second From":                  {composeOptions{}, "From: attacker@evil.example"},
		"second Subject":               {composeOptions{}, "Subject: Pay now"},
		"injected Reply-To":            {composeOptions{}, "Reply-To: attacker@evil.example"},
		"injected List-Unsubscribe":    {composeOptions{}, "List-Unsubscribe: <https://evil.example>"},
		"second List-Unsubscribe-Post": {composeOptions{unsubscribe: true}, "List-Unsubscribe-Post: List-Unsubscribe=One-Click"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			signed := sign(t, compose(t, tc.options))
			require.NoError(t, verify(t, signed))
			tampered := append([]byte(tc.header+"\r\n"), signed...)
			assert.Error(t, verify(t, tampered))
		})
	}
}

func TestSign_ModifiedBodyBreaksTheSignature(t *testing.T) {
	signed := sign(t, compose(t, composeOptions{}))
	tampered := bytes.Replace(signed, []byte("Bonjour"), []byte("Bonsoir"), 1)
	require.NotEqual(t, signed, tampered)
	assert.Error(t, verify(t, tampered))
}

func TestSign_Errors(t *testing.T) {
	key, _, _ := testKey(t)
	message := compose(t, composeOptions{})

	_, err := Sign(message, SignParams{Domain: testDomain, Selector: testSelector})
	assert.True(t, errors.Is(err, ErrSign), "no key")

	_, err = Sign(message, SignParams{Selector: testSelector, Key: key})
	assert.True(t, errors.Is(err, ErrSign), "no domain")
}

func TestHeaderKeys(t *testing.T) {
	message := []byte("From: a@acme.io\r\nTo: b@example.com\r\nTo: c@example.com\r\nSubject: x\r\n folded\r\nDate: now\r\n\r\nFrom: in the body\r\n")
	assert.Equal(t, []string{
		"From", "From", "To", "To", "Subject", "Subject", "Reply-To",
		"List-Unsubscribe", "List-Unsubscribe-Post",
	}, HeaderKeys(message))
}
