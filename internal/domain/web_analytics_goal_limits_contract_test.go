package domain

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trackGoal skips, with a console warning, a goal the server would drop, so the
// SDK restates the server's goal limits and its idea of a blank name. Neither
// side can see the other: the SDK workflow runs no Go, and the Go workflow never
// looks at the SDK. If they drift, the SDK either sends goals the server drops
// without a word or skips goals the server would keep.
//
// This test is what couples them, and it reads the SDK source for the same
// reason the click-ID contract test does.

const sdkGoalDataSourcePath = "../../web_analytics_sdk/src/utils/goal-data.ts"

// Anchored to whole lines, so a commented-out copy of a declaration is not read.
var sdkGoalLimitRE = regexp.MustCompile(`(?m)^export const (MAX_GOAL_[A-Z_]+) = ([0-9.e]+);\r?$`)

var sdkGoBlankRE = regexp.MustCompile(`(?m)^const GO_BLANK = /\^\[(.*?)\]\*\$/;\r?$`)

func readSDKGoalData(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(sdkGoalDataSourcePath))
	require.NoError(t, err,
		"cannot read the SDK's goal limits; if goal-data.ts moved, this test must follow it rather than be deleted")
	return raw
}

func TestWebAnalyticsSDKGoalLimitsMatchTheServer(t *testing.T) {
	limits := map[string]float64{}
	for _, m := range sdkGoalLimitRE.FindAllSubmatch(readSDKGoalData(t), -1) {
		name := string(m[1])
		require.NotContains(t, limits, name, "%s is declared twice in %s", name, sdkGoalDataSourcePath)
		v, err := strconv.ParseFloat(string(m[2]), 64)
		require.NoError(t, err, "%s is not a plain number", name)
		limits[name] = v
	}

	want := map[string]float64{
		"MAX_GOAL_VALUE":                WebTrackMaxGoalValue,
		"MAX_GOAL_ACTION_BYTES":         WebTrackMaxGoalNameLength,
		"MAX_GOAL_PROPERTY_KEYS":        WebTrackMaxGoalPropertyKeys,
		"MAX_GOAL_PROPERTY_VALUE_BYTES": WebTrackMaxGoalPropertyValueLength,
		"MAX_GOAL_PROPERTIES_BYTES":     WebTrackMaxGoalPropertiesBytes,
	}
	assert.Equal(t, want, limits,
		"the SDK's goal limits in %s must be the server's, each written as a plain number", sdkGoalDataSourcePath)
}

// The server drops a goal whose name strings.TrimSpace empties, which is a name
// made only of unicode.IsSpace characters.
func TestWebAnalyticsSDKBlankGoalNameMatchesTheServer(t *testing.T) {
	ms := sdkGoBlankRE.FindAllSubmatch(readSDKGoalData(t), -1)
	require.Len(t, ms, 1,
		"GO_BLANK must be declared once, as a single character class, in %s; update the parser here, and do not silently stop checking", sdkGoalDataSourcePath)

	inClass := parseJSCharClass(t, string(ms[0][1]))
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if inClass[r] != unicode.IsSpace(r) {
			assert.Failf(t, "GO_BLANK differs from unicode.IsSpace",
				"U+%04X: in GO_BLANK %v, unicode.IsSpace %v", r, inClass[r], unicode.IsSpace(r))
		}
	}
}

// parseJSCharClass reads the body of a JS regexp character class made of
// literal characters, the escapes \t \n \v \f \r and \uXXXX, and ranges.
func parseJSCharClass(t *testing.T, class string) map[rune]bool {
	t.Helper()
	src := []rune(class)
	i := 0
	next := func() rune {
		require.Less(t, i, len(src), "GO_BLANK ends mid-escape")
		r := src[i]
		i++
		if r != '\\' {
			return r
		}
		require.Less(t, i, len(src), "GO_BLANK ends mid-escape")
		e := src[i]
		i++
		switch e {
		case 't':
			return '\t'
		case 'n':
			return '\n'
		case 'v':
			return '\v'
		case 'f':
			return '\f'
		case 'r':
			return '\r'
		case 'u':
			require.LessOrEqual(t, i+4, len(src), "GO_BLANK has a short \\u escape")
			code, err := strconv.ParseUint(string(src[i:i+4]), 16, 32)
			require.NoError(t, err, "GO_BLANK has a malformed \\u escape")
			i += 4
			return rune(code)
		default:
			require.Failf(t, "unsupported escape", "GO_BLANK uses \\%c, which this parser does not read", e)
			return 0
		}
	}

	set := map[rune]bool{}
	for i < len(src) {
		lo := next()
		hi := lo
		if i < len(src)-1 && src[i] == '-' {
			i++
			hi = next()
		}
		require.LessOrEqual(t, lo, hi, "GO_BLANK has a reversed range")
		for r := lo; r <= hi; r++ {
			set[r] = true
		}
	}
	require.NotEmpty(t, set, "parsed an empty GO_BLANK class, which would make the comparison vacuous")
	return set
}
