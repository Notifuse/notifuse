package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/safehttpclient"
)

// OAuthClientDocument is a fetched Client ID Metadata Document: its body and
// the response headers that set how long it is cached.
type OAuthClientDocument struct {
	Body   []byte
	Header http.Header
}

// OAuthClientFetcher fetches a Client ID Metadata Document. Injectable, so
// that tests and the integration suite serve their own documents.
type OAuthClientFetcher interface {
	Fetch(ctx context.Context, clientID string) (*OAuthClientDocument, error)
}

// HTTPOAuthClientFetcher fetches metadata documents over the network, through
// the SSRF guard of safehttpclient: no private or reserved address, a 5 s
// timeout, no redirect followed (a 3xx is a failure), a 200 with a JSON
// content type, and a body of at most 64 KB.
type HTTPOAuthClientFetcher struct {
	client *http.Client
}

// NewHTTPOAuthClientFetcher builds the fetcher on safehttpclient's DialContext.
func NewHTTPOAuthClientFetcher() *HTTPOAuthClientFetcher {
	return newHTTPOAuthClientFetcher(&http.Transport{
		DialContext:           safehttpclient.DialContext,
		TLSHandshakeTimeout:   domain.OAuthClientFetchTimeout,
		ResponseHeaderTimeout: domain.OAuthClientFetchTimeout,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
	})
}

// newHTTPOAuthClientFetcher builds the fetcher on a transport, which tests
// replace to reach a local server.
func newHTTPOAuthClientFetcher(transport http.RoundTripper) *HTTPOAuthClientFetcher {
	return &HTTPOAuthClientFetcher{client: &http.Client{
		Timeout:   domain.OAuthClientFetchTimeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

// Fetch gets the document at clientID.
func (f *HTTPOAuthClientFetcher) Fetch(ctx context.Context, clientID string) (*OAuthClientDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clientID, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid client_id URL: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		if errors.Is(err, safehttpclient.ErrPrivateIP) {
			return nil, fmt.Errorf("the client_id host resolves to a private or reserved address: %w", err)
		}
		return nil, fmt.Errorf("the client metadata document could not be fetched: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the client metadata document answered %d, not 200", resp.StatusCode)
	}
	if !isJSONContentType(resp.Header.Get("Content-Type")) {
		return nil, fmt.Errorf("the client metadata document is not served as JSON")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, domain.OAuthClientMaxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("the client metadata document could not be read: %w", err)
	}
	if len(body) > domain.OAuthClientMaxBodyBytes {
		return nil, fmt.Errorf("the client metadata document is larger than %d bytes", domain.OAuthClientMaxBodyBytes)
	}
	return &OAuthClientDocument{Body: body, Header: resp.Header}, nil
}

// isJSONContentType accepts application/json and the +json types.
func isJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/json" || (strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json"))
}

// oauthClientCacheTTL reads how long a fetched document may be cached from its
// headers (Cache-Control max-age, else Expires), within the bounds of
// OAuthClientMinCache and OAuthClientMaxCache. no-store, no-cache, and no
// header at all give the lower bound.
func oauthClientCacheTTL(header http.Header, now time.Time) time.Duration {
	ttl := time.Duration(0)
	found := false
	for _, directive := range strings.Split(header.Get("Cache-Control"), ",") {
		directive = strings.ToLower(strings.TrimSpace(directive))
		switch {
		case directive == "no-store" || directive == "no-cache":
			return domain.OAuthClientMinCache
		case strings.HasPrefix(directive, "max-age="):
			if seconds, err := strconv.ParseInt(strings.Trim(strings.TrimPrefix(directive, "max-age="), `"`), 10, 64); err == nil && seconds >= 0 {
				ttl = domain.OAuthClientMaxCache + time.Second
				if seconds <= int64(domain.OAuthClientMaxCache/time.Second) {
					ttl = time.Duration(seconds) * time.Second
				}
				found = true
			}
		}
	}
	if !found {
		if expires, err := http.ParseTime(header.Get("Expires")); err == nil {
			ttl = expires.Sub(now)
			found = true
		}
	}
	if !found || ttl < domain.OAuthClientMinCache {
		return domain.OAuthClientMinCache
	}
	if ttl > domain.OAuthClientMaxCache {
		return domain.OAuthClientMaxCache
	}
	return ttl
}
