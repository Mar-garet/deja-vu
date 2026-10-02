// Command testshard prints the -run and -skip patterns that give one CI shard
// its slice of the test suite.
//
// cmd/deja alone is over three thousand tests in one binary, and a package's
// tests run one after another, so the test job was as long as that package no
// matter how many cores the runner had. Splitting it means splitting by test
// name, and a list of a thousand names in -run is longer than a windows
// command line. So a shard is a range of names instead: every test, fuzz
// target and example in the repository is sorted, cut into N runs of equal
// count, and shard i gets the names from its own first name up to the next
// shard's first name — `-run` matches "at or after the start", `-skip` matches
// "at or after the end". Each pattern is a few hundred bytes.
//
// The ranges cover every possible name, not just the ones found here, so a test
// this scanner misses still runs in exactly one shard; the count only decides
// where the cuts fall.
//
//	go run ./scripts/testshard -shard 2 -of 3 >> "$GITHUB_OUTPUT"
//	go test -run "$RUN" -skip "$SKIP" ./...
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

func main() {
	shard := flag.Int("shard", 1, "which shard, counting from 1")
	of := flag.Int("of", 1, "how many shards")
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	if *of < 1 || *shard < 1 || *shard > *of {
		fmt.Fprintf(os.Stderr, "testshard: -shard %d -of %d is not a shard\n", *shard, *of)
		os.Exit(2)
	}
	names, err := testNames(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "testshard:", err)
		os.Exit(1)
	}
	run, skip := patterns(names, *shard, *of)
	fmt.Printf("run=%s\nskip=%s\n", run, skip)
}

// topLevel is a top-level test, fuzz target or example. A method cannot match:
// its receiver puts a parenthesis right after func.
var topLevel = regexp.MustCompile(`(?m)^func ((?:Test|Fuzz|Example)[^\s(\[]*)[(\[]`)

// testNames returns every distinct test function name in the module's
// _test.go files, sorted. Build constraints are ignored on purpose: every
// runner must cut at the same names whatever its GOOS.
func testNames(root string) ([]string, error) {
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range topLevel.FindAllSubmatch(src, -1) {
			seen[string(m[1])] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// patterns returns the -run and -skip values for shard i of n over the sorted
// names. An empty value is go test's own default: run everything, skip nothing.
func patterns(names []string, i, n int) (run, skip string) {
	start := func(k int) string { return names[len(names)*k/n] }
	if len(names) == 0 {
		if i == 1 {
			return "", ""
		}
		// Nothing to cut at: the first shard takes it all.
		return "^$", ""
	}
	if i > 1 {
		run = atOrAfter(start(i - 1))
	}
	if i < n {
		skip = atOrAfter(start(i))
	}
	return run, skip
}

// atOrAfter returns a pattern matching exactly the names that sort at or after
// p, in Go's byte order (which for UTF-8 is code point order). At each rune the
// name either carries on with p's rune, or has a greater one there and anything
// after it; having all of p is enough.
func atOrAfter(p string) string {
	return "^" + rest([]rune(p))
}

func rest(p []rune) string {
	if len(p) == 0 {
		return ""
	}
	r := p[0]
	alt := regexp.QuoteMeta(string(r)) + rest(p[1:])
	if r < utf8.MaxRune {
		alt += fmt.Sprintf(`|[\x{%x}-\x{10ffff}]`, r+1)
	}
	return "(?:" + alt + ")"
}
