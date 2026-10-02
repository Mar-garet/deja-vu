package main

import (
	"math/rand"
	"regexp"
	"testing"
)

// shard is how go test reads the pair: run a test if -run matches (or is
// empty) and -skip does not.
type shard struct{ run, skip *regexp.Regexp }

func compile(run, skip string) shard {
	var s shard
	if run != "" {
		s.run = regexp.MustCompile(run)
	}
	if skip != "" {
		s.skip = regexp.MustCompile(skip)
	}
	return s
}

func (s shard) has(name string) bool {
	if s.run != nil && !s.run.MatchString(name) {
		return false
	}
	return s.skip == nil || !s.skip.MatchString(name)
}

func TestEveryNameLandsInExactlyOneShard(t *testing.T) {
	names, err := testNames("../..")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 1000 {
		t.Fatalf("found %d test names in the repository; the scanner is missing most of them", len(names))
	}
	// Names the scanner never saw must land somewhere too: a test it misses
	// still has to run.
	probe := append([]string{}, names...)
	rng := rand.New(rand.NewSource(1))
	const alphabet = "09AZ_azé日"
	for i := 0; i < 2000; i++ {
		b := []rune(names[rng.Intn(len(names))])
		b = b[:rng.Intn(len(b)+1)]
		for j := rng.Intn(4); j > 0; j-- {
			b = append(b, []rune(alphabet)[rng.Intn(len([]rune(alphabet)))])
		}
		probe = append(probe, string(b))
	}
	probe = append(probe, "", "Test", "\U0010ffff")

	for _, n := range []int{1, 2, 3, 4} {
		shards := make([]shard, n)
		for i := range shards {
			shards[i] = compile(patterns(names, i+1, n))
		}
		for _, name := range probe {
			hit := 0
			for _, s := range shards {
				if s.has(name) {
					hit++
				}
			}
			if hit != 1 {
				t.Fatalf("of %d shards, %q is in %d", n, name, hit)
			}
		}
		counts := make([]int, n)
		for _, name := range names {
			for i, s := range shards {
				if s.has(name) {
					counts[i]++
				}
			}
		}
		for i, c := range counts {
			if want := len(names) / n; c < want-1 || c > want+n {
				t.Errorf("of %d shards, shard %d has %d of %d names", n, i+1, c, len(names))
			}
		}
	}
}

func TestAtOrAfterAgreesWithStringOrder(t *testing.T) {
	words := []string{"", "A", "Test", "TestA", "TestB", "TestAb", "TestA_", "TestZ", "Testa", "Testé", "Test9", "TestX.Y"}
	for _, p := range words {
		re := regexp.MustCompile(atOrAfter(p))
		for _, w := range words {
			if got, want := re.MatchString(w), w >= p; got != want {
				t.Errorf("atOrAfter(%q) on %q = %v, want %v", p, w, got, want)
			}
		}
	}
}

func TestTopLevelSkipsMethodsAndHelpers(t *testing.T) {
	src := "func TestA(t *testing.T) {}\nfunc (s) TestB() {}\nfunc helper() {}\nfunc FuzzC(f *testing.F) {}\nfunc ExampleD() {}\n  func TestIndented() {}\nfunc TestG[T any](t *testing.T) {}\n"
	var got []string
	for _, m := range topLevel.FindAllStringSubmatch(src, -1) {
		got = append(got, m[1])
	}
	want := []string{"TestA", "FuzzC", "ExampleD", "TestG"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
