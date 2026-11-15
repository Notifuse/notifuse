// Command shard splits the integration suite across CI jobs.
//
// The suite is one package whose tests run one after the other, and a few of them
// take minutes, so CI runs it as several jobs, each on its own Postgres and Mailpit.
// This tool decides which job runs which test:
//
//	integ.test -test.list '.*' | shard assign -shards 4 -index 0 -durations durations.json
//
// prints the -test.run pattern for job 0, and -format names prints its tests one per
// line (what the job checks it ran). Every listed test lands in exactly one job,
// whether or not durations.json knows it: the file only weighs the split. A test it
// does not know weighs the median, so a stale file unbalances jobs, never drops a test.
//
//	go test -json ... | shard durations > durations.json
//
// rebuilds that file from a run of the suite.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "shard:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: shard assign|durations [flags]")
	}
	switch args[0] {
	case "assign":
		return runAssign(args[1:], stdin, stdout)
	case "durations":
		durations, err := parseTestEvents(stdin)
		if err != nil {
			return err
		}
		out, err := json.MarshalIndent(durations, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "%s\n", out)
		return err
	default:
		return fmt.Errorf("unknown command %q (want assign or durations)", args[0])
	}
}

func runAssign(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("assign", flag.ContinueOnError)
	shards := fs.Int("shards", 0, "number of jobs")
	index := fs.Int("index", -1, "this job's index, from 0")
	durationsPath := fs.String("durations", "", "JSON file of test name -> seconds (optional)")
	format := fs.String("format", "regex", "regex (a -test.run pattern) or names (one per line)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *shards < 1 {
		return fmt.Errorf("-shards must be at least 1, got %d", *shards)
	}
	if *index < 0 || *index >= *shards {
		return fmt.Errorf("-index must be in [0, %d), got %d", *shards, *index)
	}

	names, err := parseNames(stdin)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return errors.New("no tests listed on stdin")
	}

	durations := map[string]float64{}
	if *durationsPath != "" {
		raw, err := os.ReadFile(*durationsPath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &durations); err != nil {
			return fmt.Errorf("%s: %w", *durationsPath, err)
		}
	}

	mine := assign(names, durations, *shards)[*index]
	switch *format {
	case "regex":
		_, err = fmt.Fprintln(stdout, runPattern(mine))
	case "names":
		for _, name := range mine {
			if _, err = fmt.Fprintln(stdout, name); err != nil {
				break
			}
		}
	default:
		err = fmt.Errorf("unknown -format %q (want regex or names)", *format)
	}
	return err
}

var testName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parseNames reads -test.list output. Anything that is not a function name is an
// error rather than skipped: a test binary printing more than names would mean the
// list cannot be trusted to be the whole suite.
func parseNames(r io.Reader) ([]string, error) {
	var names []string
	seen := map[string]bool{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !testName.MatchString(line) {
			return nil, fmt.Errorf("unexpected line in the test list: %q", line)
		}
		if !seen[line] {
			seen[line] = true
			names = append(names, line)
		}
	}
	return names, scanner.Err()
}

// assign splits names into shards, heaviest test first onto the lightest shard.
// The result depends only on its inputs, so every job computes the same split.
func assign(names []string, durations map[string]float64, shards int) [][]string {
	fallback := median(durations)
	weight := func(name string) float64 {
		if d, ok := durations[name]; ok && d > 0 {
			return d
		}
		return fallback
	}

	ordered := append([]string(nil), names...)
	sort.Slice(ordered, func(i, j int) bool {
		wi, wj := weight(ordered[i]), weight(ordered[j])
		if wi != wj {
			return wi > wj
		}
		return ordered[i] < ordered[j]
	})

	out := make([][]string, shards)
	load := make([]float64, shards)
	for _, name := range ordered {
		lightest := 0
		for s := 1; s < shards; s++ {
			if load[s] < load[lightest] {
				lightest = s
			}
		}
		out[lightest] = append(out[lightest], name)
		load[lightest] += weight(name)
	}
	for _, shard := range out {
		sort.Strings(shard)
	}
	return out
}

func median(durations map[string]float64) float64 {
	var values []float64
	for _, d := range durations {
		if d > 0 {
			values = append(values, d)
		}
	}
	if len(values) == 0 {
		return 1
	}
	sort.Float64s(values)
	return values[len(values)/2]
}

// runPattern anchors each name so TestFoo does not also run TestFooBar. A shard
// with nothing to run gets a pattern no test name matches.
func runPattern(names []string) string {
	if len(names) == 0 {
		return "^$"
	}
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = regexp.QuoteMeta(name)
	}
	return "^(" + strings.Join(quoted, "|") + ")$"
}

// minDuration is the weight of a test that ran in under a tenth of a second.
const minDuration = 0.1

type testEvent struct {
	Action  string
	Test    string
	Elapsed float64
}

// parseTestEvents reads `go test -json` output and keeps each top-level test's
// elapsed time, rounded to a tenth of a second (at least minDuration). Lines that are not JSON (the build's
// own output) are skipped; subtests are skipped, since -test.run selects top-level
// tests only.
func parseTestEvents(r io.Reader) (map[string]float64, error) {
	durations := map[string]float64{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var ev testEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Test == "" || strings.Contains(ev.Test, "/") {
			continue
		}
		if ev.Action == "pass" || ev.Action == "fail" {
			// Never 0: assign reads 0 as "unknown" and would weigh an instant test
			// at the median.
			durations[ev.Test] = math.Max(math.Round(ev.Elapsed*10)/10, minDuration)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(durations) == 0 {
		return nil, errors.New("no test results in the input (is it `go test -json` output?)")
	}
	return durations, nil
}
