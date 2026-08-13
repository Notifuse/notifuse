package domain

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The click-ID contract spans two languages and two CI workflows.
//
// The browser SDK decides which URL parameters count as ad click ids and reports
// the one it found in utm_id_from. The seeded attribution rules in Go match on
// that field. Neither side can see the other: the SDK workflow runs no Go, and
// the Go workflow never looks at the SDK.
//
// The result was drift in both directions — four seeded rules matching ids the
// SDK never captured, and one captured id no rule matched. Nothing failed. The
// rules simply never fired, and the traffic they were meant to label stayed
// unattributed, which looks like "we get no Pinterest traffic" rather than like a
// bug.
//
// This test is the only thing that couples the two lists. It reads the SDK source
// directly for that reason: importing a generated copy, or restating the list in
// Go, would just move the drift somewhere quieter.

const sdkUTMSourcePath = "../../web_analytics_sdk/src/utils/utm.ts"

var sdkClickIDArrayRE = regexp.MustCompile(`(?s)export const DEFAULT_AD_CLICK_IDS\s*=\s*\[(.*?)\]`)
var sdkClickIDEntryRE = regexp.MustCompile(`'([^']+)'`)

// sdkDefaultClickIDs parses DEFAULT_AD_CLICK_IDS out of the SDK source.
func sdkDefaultClickIDs(t *testing.T) []string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Clean(sdkUTMSourcePath))
	require.NoError(t, err,
		"cannot read the SDK's click-id list; if utm.ts moved, this test must follow it rather than be deleted")

	block := sdkClickIDArrayRE.FindSubmatch(raw)
	require.NotNil(t, block,
		"DEFAULT_AD_CLICK_IDS is no longer a plain array literal in %s — update the parser here, and do not silently stop checking", sdkUTMSourcePath)

	var ids []string
	for _, m := range sdkClickIDEntryRE.FindAllSubmatch(block[1], -1) {
		ids = append(ids, string(m[1]))
	}
	require.NotEmpty(t, ids, "parsed an empty click-id list, which would make every assertion below vacuous")
	return ids
}

// clickIDMatchers returns every utm_id_from condition across the seeded rules,
// keyed by the rule that carries it.
func clickIDMatchers(t *testing.T) map[string]WebFilterCondition {
	t.Helper()

	out := map[string]WebFilterCondition{}
	for _, filter := range DefaultWebFilters() {
		for _, cond := range filter.Conditions {
			if cond.Field == "utm_id_from" && cond.Operator != WebFilterOpIsEmpty {
				out[filter.Name] = cond
			}
		}
	}
	require.NotEmpty(t, out, "no seeded rule matches on utm_id_from at all")
	return out
}

func conditionMatches(t *testing.T, cond WebFilterCondition, value string) bool {
	t.Helper()
	switch cond.Operator {
	case WebFilterOpEquals:
		return cond.Value == value
	case WebFilterOpRegex:
		re, err := regexp.Compile(cond.Value)
		require.NoError(t, err, "seeded rule carries an uncompilable regex: %q", cond.Value)
		return re.MatchString(value)
	case WebFilterOpContains:
		return strings.Contains(value, cond.Value)
	default:
		t.Fatalf("unhandled operator %q in a utm_id_from condition — extend this test", cond.Operator)
		return false
	}
}

// Every id the SDK captures must be attributable. An id with no rule behind it
// produces traffic labelled "not-mapped" that nobody can explain.
func TestEverySDKClickIDHasASeededRule(t *testing.T) {
	matchers := clickIDMatchers(t)

	for _, id := range sdkDefaultClickIDs(t) {
		t.Run(id, func(t *testing.T) {
			for _, cond := range matchers {
				if conditionMatches(t, cond, id) {
					return
				}
			}
			t.Errorf("the SDK captures %q but no seeded rule matches it, so that traffic stays unattributed", id)
		})
	}
}

// And every rule must be reachable. A rule matching an id the SDK never sends is
// dead by construction: it cannot fire, ever, and its absence from reports reads
// as "no traffic from this network".
func TestEverySeededClickIDRuleIsReachable(t *testing.T) {
	ids := sdkDefaultClickIDs(t)

	for name, cond := range clickIDMatchers(t) {
		t.Run(name, func(t *testing.T) {
			for _, id := range ids {
				if conditionMatches(t, cond, id) {
					return
				}
			}
			t.Errorf("rule %q matches utm_id_from %q %q, which the SDK never captures — the rule can never fire",
				name, cond.Operator, cond.Value)
		})
	}
}

// The parser above is the weak point of this file: if it silently matched
// nothing, both tests would pass having checked nothing at all. This asserts it
// finds a list of a plausible shape.
func TestSDKClickIDParserActuallyParses(t *testing.T) {
	ids := sdkDefaultClickIDs(t)

	assert.GreaterOrEqual(t, len(ids), 9, "the list has only ever grown")
	assert.Contains(t, ids, "gclid", "the most common click id of all is missing — the parser is probably broken")
	for _, id := range ids {
		assert.NotContains(t, id, " ", "%q looks like prose, not a URL parameter", id)
		assert.NotEmpty(t, id)
	}
}
