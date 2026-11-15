package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func names(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("Test%03d", i)
	}
	return out
}

func TestAssign_EveryTestInExactlyOneShard(t *testing.T) {
	all := names(41)
	durations := map[string]float64{"Test000": 600, "Test001": 70, "Test007": 0.5}
	for shards := 1; shards <= 8; shards++ {
		got := assign(all, durations, shards)
		require.Len(t, got, shards)
		var union []string
		for _, shard := range got {
			union = append(union, shard...)
		}
		sort.Strings(union)
		assert.Equal(t, all, union, "%d shards: union is the input, no test twice", shards)
	}
}

func TestAssign_TestsMissingFromDurationsAreStillAssigned(t *testing.T) {
	got := assign([]string{"TestKnown", "TestNew"}, map[string]float64{"TestKnown": 3}, 2)
	assert.ElementsMatch(t, []string{"TestKnown", "TestNew"}, append(got[0], got[1]...))

	got = assign([]string{"TestA", "TestB", "TestC"}, nil, 2)
	assert.Len(t, append(got[0], got[1]...), 3, "no durations file at all")
}

func TestAssign_IsDeterministic(t *testing.T) {
	all := names(30)
	durations := map[string]float64{"Test003": 10, "Test004": 10, "Test010": 5}
	first := assign(all, durations, 4)
	reversed := append([]string(nil), all...)
	sort.Sort(sort.Reverse(sort.StringSlice(reversed)))
	for i := 0; i < 5; i++ {
		assert.Equal(t, first, assign(all, durations, 4))
		assert.Equal(t, first, assign(reversed, durations, 4), "input order does not matter")
	}
}

func TestAssign_HeaviestTestIsAloneWhenItOutweighsTheRest(t *testing.T) {
	durations := map[string]float64{"TestHuge": 600}
	all := []string{"TestHuge"}
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("TestSmall%02d", i)
		all = append(all, name)
		durations[name] = 2
	}
	got := assign(all, durations, 4)
	holding := 0
	for _, shard := range got {
		if contains(shard, "TestHuge") {
			holding++
			assert.Equal(t, []string{"TestHuge"}, shard)
		}
	}
	assert.Equal(t, 1, holding, "exactly one shard runs TestHuge")
}

func TestAssign_BalancesLoad(t *testing.T) {
	durations := map[string]float64{}
	var all []string
	for i, d := range []float64{60, 50, 40, 30, 20, 10, 10, 10, 5, 5, 5, 5} {
		name := fmt.Sprintf("Test%02d", i)
		all = append(all, name)
		durations[name] = d
	}
	got := assign(all, durations, 3)
	var loads []float64
	for _, shard := range got {
		sum := 0.0
		for _, name := range shard {
			sum += durations[name]
		}
		loads = append(loads, sum)
	}
	sort.Float64s(loads)
	assert.LessOrEqual(t, loads[2]-loads[0], 10.0, "loads %v", loads)
}

func TestRunPattern(t *testing.T) {
	pattern := runPattern([]string{"TestFoo", "TestBar"})
	re := regexp.MustCompile(pattern)
	assert.True(t, re.MatchString("TestFoo"))
	assert.True(t, re.MatchString("TestBar"))
	assert.False(t, re.MatchString("TestFooBar"), "anchored: a prefix does not run its longer namesake")
	assert.False(t, re.MatchString("XTestFoo"))

	empty := regexp.MustCompile(runPattern(nil))
	assert.False(t, empty.MatchString("TestFoo"), "an empty shard runs nothing")
}

func TestParseNames(t *testing.T) {
	got, err := parseNames(strings.NewReader("TestB\n\nTestA\n  TestC  \nTestA\n"))
	require.NoError(t, err)
	assert.Equal(t, []string{"TestB", "TestA", "TestC"}, got)

	_, err = parseNames(strings.NewReader("TestA\nok  \tgithub.com/x\t0.1s\n"))
	assert.Error(t, err, "anything but a name means the list is not the suite")
}

func TestParseTestEvents(t *testing.T) {
	input := strings.Join([]string{
		`# github.com/Notifuse/notifuse/tests/integration`,
		`{"Action":"run","Test":"TestA"}`,
		`{"Action":"output","Test":"TestA","Output":"--- PASS: TestA (12.34s)\n"}`,
		`{"Action":"pass","Test":"TestA/sub","Elapsed":5}`,
		`{"Action":"pass","Test":"TestA","Elapsed":12.34}`,
		`{"Action":"fail","Test":"TestB","Elapsed":0.06}`,
		`{"Action":"pass","Test":"TestInstant","Elapsed":0.01}`,
		`{"Action":"skip","Test":"TestC","Elapsed":0}`,
		`{"Action":"pass","Elapsed":99}`,
	}, "\n")
	got, err := parseTestEvents(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"TestA": 12.3, "TestB": 0.1, "TestInstant": 0.1}, got,
		"an instant test weighs the minimum, not 0 (which assign would read as unknown)")

	_, err = parseTestEvents(strings.NewReader("not json\n"))
	assert.Error(t, err)
}

func TestRun_AssignEndToEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "durations.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"TestSlow": 100, "TestFast": 1, "TestMid": 2}`), 0o600))
	list := "TestSlow\nTestFast\nTestMid\nTestOther\n"

	var all []string
	for i := 0; i < 2; i++ {
		var out bytes.Buffer
		err := run([]string{"assign", "-shards", "2", "-index", fmt.Sprint(i), "-durations", path, "-format", "names"},
			strings.NewReader(list), &out)
		require.NoError(t, err)
		all = append(all, strings.Fields(out.String())...)
	}
	assert.ElementsMatch(t, []string{"TestSlow", "TestFast", "TestMid", "TestOther"}, all)

	var out bytes.Buffer
	require.NoError(t, run([]string{"assign", "-shards", "2", "-index", "0", "-durations", path},
		strings.NewReader(list), &out))
	assert.Equal(t, "^(TestSlow)$\n", out.String(), "the 100s test fills shard 0 on its own")

	for _, bad := range [][]string{
		{"assign", "-shards", "0", "-index", "0"},
		{"assign", "-shards", "2", "-index", "2"},
		{"assign", "-shards", "2", "-index", "0", "-format", "xml"},
		{"assign", "-shards", "2", "-index", "0", "-durations", filepath.Join(dir, "missing.json")},
		{"unknown"},
		{},
	} {
		assert.Error(t, run(bad, strings.NewReader(list), &bytes.Buffer{}), "%v", bad)
	}
	assert.Error(t, run([]string{"assign", "-shards", "1", "-index", "0"}, strings.NewReader(""), &bytes.Buffer{}),
		"an empty list is an error, not a job that runs nothing")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
