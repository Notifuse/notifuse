package http

import (
	"net/http"
	"strings"
)

// maxForwardedHosts caps the forwarded hosts a click compares its link with. Proxies
// append their entry, so the last ones are those of the proxies nearest the server,
// which a client cannot push out by sending more.
const maxForwardedHosts = 8

// forwardedHosts returns the hosts a reverse proxy forwarded: every X-Forwarded-Host
// entry, then the host parameter of every Forwarded element (RFC 7239), in header order,
// trimmed and without empty values.
func forwardedHosts(h http.Header) []string {
	var hosts []string
	for _, value := range h.Values("X-Forwarded-Host") {
		for _, entry := range strings.Split(value, ",") {
			if entry = strings.TrimSpace(entry); entry != "" {
				hosts = append(hosts, entry)
			}
		}
	}
	for _, value := range h.Values("Forwarded") {
		hosts = append(hosts, forwardedHeaderHosts(value)...)
	}
	return hosts
}

// forwardedHeaderHosts reads the non-empty host parameters of one Forwarded header
// value: elements separated by commas, pairs by semicolons, a quoted value unquoted.
// Separators inside a quoted value do not count.
func forwardedHeaderHosts(value string) []string {
	var hosts []string
	var name, current strings.Builder
	inValue, quoted, escaped := false, false, false
	flush := func() {
		if inValue && strings.EqualFold(strings.TrimSpace(name.String()), "host") {
			if v := strings.TrimSpace(current.String()); v != "" {
				hosts = append(hosts, v)
			}
		}
		name.Reset()
		current.Reset()
		inValue = false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case escaped:
			current.WriteByte(c)
			escaped = false
		case quoted && c == '\\':
			escaped = true
		case c == '"' && inValue:
			quoted = !quoted
		case quoted:
			current.WriteByte(c)
		case c == ',' || c == ';':
			flush()
		case c == '=' && !inValue:
			inValue = true
		case inValue:
			current.WriteByte(c)
		default:
			name.WriteByte(c)
		}
	}
	flush()
	return hosts
}
