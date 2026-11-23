package dkim

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Result is what a lookup of a key's TXT record found.
type Result string

const (
	// ResultOK: one DKIM record, carrying the expected public key.
	ResultOK Result = "ok"
	// ResultNotFound: no TXT record at the name (NXDOMAIN or no data).
	ResultNotFound Result = "not_found"
	// ResultMismatch: a record is there, but it is invalid, revoked, duplicated or
	// carries another key.
	ResultMismatch Result = "mismatch"
	// ResultDNSError: the lookup failed for a reason that may pass (timeout, SERVFAIL).
	ResultDNSError Result = "dns_error"
)

// LookupTimeout bounds one TXT lookup.
const LookupTimeout = 10 * time.Second

// TXTResolver looks up the TXT records of a name. *net.Resolver satisfies it.
type TXTResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// DefaultResolver is the system resolver.
var DefaultResolver TXTResolver = &net.Resolver{}

// RecordName is the name a key's TXT record is published at, without the final dot.
func RecordName(selector, domain string) string {
	return selector + "._domainkey." + domain
}

// VerifyDNS looks up the record of selector on domain and compares the key it publishes
// with expectedPublicKeyB64. The name is queried fully qualified, so no local search
// suffix is tried. detail says why the result is not ok.
func VerifyDNS(ctx context.Context, resolver TXTResolver, selector, domain, expectedPublicKeyB64 string) (Result, string) {
	expected, err := ParsePublicKeyDER(expectedPublicKeyB64)
	if err != nil {
		return ResultMismatch, "the stored public key cannot be read: " + err.Error()
	}

	ctx, cancel := context.WithTimeout(ctx, LookupTimeout)
	defer cancel()
	records, err := resolver.LookupTXT(ctx, RecordName(selector, domain)+".")
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return ResultNotFound, "no TXT record found"
		}
		return ResultDNSError, "DNS lookup failed: " + err.Error()
	}
	if len(records) == 0 {
		return ResultNotFound, "no TXT record found"
	}
	if len(records) > 1 {
		return ResultMismatch, fmt.Sprintf("%d TXT records found, a selector must publish one", len(records))
	}

	tags, err := parseTags(records[0])
	if err != nil {
		return ResultMismatch, err.Error()
	}
	if err := checkTags(tags); err != nil {
		return ResultMismatch, err.Error()
	}
	published, err := ParsePublicKeyDER(tags.value("p"))
	if err != nil {
		return ResultMismatch, "the published key cannot be read: " + err.Error()
	}
	if published.N.Cmp(expected.N) != 0 || published.E != expected.E {
		return ResultMismatch, "the published key is not this domain's key"
	}
	return ResultOK, ""
}

type tag struct{ name, value string }

type tagList []tag

func (l tagList) value(name string) string {
	for _, t := range l {
		if t.name == name {
			return t.value
		}
	}
	return ""
}

func (l tagList) has(name string) bool {
	for _, t := range l {
		if t.name == name {
			return true
		}
	}
	return false
}

// parseTags reads a tag=value list (RFC 6376 §3.2), refusing a malformed entry or a tag
// given twice.
func parseTags(record string) (tagList, error) {
	var tags tagList
	seen := map[string]bool{}
	for _, part := range strings.Split(record, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("malformed tag %q", part)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("malformed tag %q", part)
		}
		if seen[name] {
			return nil, fmt.Errorf("tag %s appears twice", name)
		}
		seen[name] = true
		tags = append(tags, tag{name: name, value: strings.TrimSpace(value)})
	}
	return tags, nil
}

// checkTags applies RFC 6376 §3.6.1 to a key record Notifuse signs with rsa-sha256.
func checkTags(tags tagList) error {
	if tags.has("v") {
		if tags[0].name != "v" {
			return errors.New("the v tag must come first")
		}
		if tags[0].value != "DKIM1" {
			return fmt.Errorf("unsupported version v=%s", tags[0].value)
		}
	}
	if tags.has("k") && tags.value("k") != "rsa" {
		return fmt.Errorf("unsupported key type k=%s", tags.value("k"))
	}
	if tags.has("h") && !listContains(tags.value("h"), "sha256") {
		return fmt.Errorf("the record does not allow sha256 (h=%s)", tags.value("h"))
	}
	if tags.has("s") && !listContains(tags.value("s"), "email") && !listContains(tags.value("s"), "*") {
		return fmt.Errorf("the record does not allow email (s=%s)", tags.value("s"))
	}
	if !tags.has("p") {
		return errors.New("the record has no p tag")
	}
	if stripWhitespace(tags.value("p")) == "" {
		return errors.New("the key is revoked (empty p tag)")
	}
	return nil
}

func listContains(list, want string) bool {
	for _, item := range strings.Split(list, ":") {
		if strings.TrimSpace(item) == want {
			return true
		}
	}
	return false
}
