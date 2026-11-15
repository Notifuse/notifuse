package http

import (
	"net/http"
	"strings"
)

// maxForwardedHosts caps the hosts a click takes from each forwarded header. Proxies
// append their entry to the header they set, so its last entries are those of the proxies
// nearest the server, which a client cannot push out by sending more. The cap applies to
// each header on its own: a proxy setting X-Forwarded-Host and passing Forwarded through
// keeps its entry whatever Forwarded values the client sends, and the reverse.
const maxForwardedHosts = 8

// forwardedHosts returns the hosts a reverse proxy forwarded: the last maxForwardedHosts
// X-Forwarded-Host entries, then the host parameters of the last maxForwardedHosts
// Forwarded elements (RFC 7239), in header order, trimmed and without empty values.
func forwardedHosts(h http.Header) []string {
	var xfh, fwd []string
	for _, value := range h.Values("X-Forwarded-Host") {
		for _, entry := range strings.Split(value, ",") {
			if entry = strings.TrimSpace(entry); entry != "" {
				xfh = append(xfh, entry)
			}
		}
	}
	for _, value := range h.Values("Forwarded") {
		fwd = append(fwd, forwardedHeaderHosts(value)...)
	}
	return append(lastHosts(xfh), lastHosts(fwd)...)
}

// lastHosts returns the last maxForwardedHosts of hosts.
func lastHosts(hosts []string) []string {
	if len(hosts) > maxForwardedHosts {
		return hosts[len(hosts)-maxForwardedHosts:]
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
