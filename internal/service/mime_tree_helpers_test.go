package service

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// mimePart is one node of a parsed MIME message: a leaf with its decoded body, or a
// multipart container with its children.
type mimePart struct {
	MediaType string
	Params    map[string]string
	Header    textproto.MIMEHeader
	Body      string
	Children  []mimePart
}

// shape renders the tree as e.g. "multipart/mixed[multipart/alternative[text/plain,text/html],application/pdf]",
// so a test can pin a whole structure in one assertion.
func (p mimePart) shape() string {
	if len(p.Children) == 0 {
		return p.MediaType
	}
	parts := make([]string, len(p.Children))
	for i, c := range p.Children {
		parts[i] = c.shape()
	}
	return p.MediaType + "[" + strings.Join(parts, ",") + "]"
}

// find returns the first leaf with the given media type, depth first.
func (p mimePart) find(mediaType string) (mimePart, bool) {
	if len(p.Children) == 0 {
		return p, p.MediaType == mediaType
	}
	for _, c := range p.Children {
		if found, ok := c.find(mediaType); ok {
			return found, true
		}
	}
	return mimePart{}, false
}

// parseMIMEMessage parses a raw RFC 5322 message into its MIME tree, decoding
// quoted-printable and base64 leaves and normalising line endings to \n.
func parseMIMEMessage(t *testing.T, raw []byte) mimePart {
	t.Helper()
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	require.NoError(t, err)
	body, err := io.ReadAll(msg.Body)
	require.NoError(t, err)
	return parseMIMEPart(t, textproto.MIMEHeader(msg.Header), body)
}

func parseMIMEPart(t *testing.T, header textproto.MIMEHeader, body []byte) mimePart {
	t.Helper()
	contentType := header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain"
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)

	part := mimePart{MediaType: mediaType, Params: params, Header: header}
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			child, err := reader.NextRawPart()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			childBody, err := io.ReadAll(child)
			require.NoError(t, err)
			part.Children = append(part.Children, parseMIMEPart(t, child.Header, childBody))
		}
		return part
	}

	switch strings.ToLower(header.Get("Content-Transfer-Encoding")) {
	case "quoted-printable":
		decoded, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(body)))
		require.NoError(t, err)
		body = decoded
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(body)), ""))
		require.NoError(t, err)
		body = decoded
	}
	part.Body = strings.ReplaceAll(string(body), "\r\n", "\n")
	return part
}
