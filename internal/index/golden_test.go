package index

import (
	"encoding/json"
	"flag"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// The golden files pin the search index format and the ranking. The same
// files are checked by src/search/engine.test.ts, which keeps the Go and
// TypeScript search implementations in agreement.
//
// Regenerate with: go test ./internal/index -run Golden -update
var update = flag.Bool("update", false, "rewrite golden files")

var goldenQueries = []string{"re", "co", "in", "install", "config", "while loop", "loop", "for", "frob", "timeout retries", "the", "gadget", "installation guide"}

type goldenResult struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

func TestGoldenSearch(t *testing.T) {
	si := BuildSearchIndex(sampleDocs())
	got := map[string][]goldenResult{}
	for _, q := range goldenQueries {
		res := si.Search(q, SearchOptions{})
		list := []goldenResult{}
		for _, r := range res {
			list = append(list, goldenResult{ID: r.Doc.ID, Score: math.Round(r.Score*1e6) / 1e6})
		}
		got[q] = list
	}
	indexPath := filepath.Join("testdata", "golden_index.json")
	searchPath := filepath.Join("testdata", "golden_search.json")
	if *update {
		writeGolden(t, indexPath, si)
		writeGolden(t, searchPath, got)
	}
	var wantIndex, gotIndex any
	readGolden(t, indexPath, &wantIndex)
	b, _ := json.Marshal(si)
	_ = json.Unmarshal(b, &gotIndex)
	if !jsonEqual(wantIndex, gotIndex) {
		t.Errorf("search index format changed; if intended, run with -update and bump FormatVersion if incompatible")
	}
	var want map[string][]goldenResult
	readGolden(t, searchPath, &want)
	for _, q := range goldenQueries {
		w, g := want[q], got[q]
		if len(w) != len(g) {
			t.Errorf("%q: %d results, want %d", q, len(g), len(w))
			continue
		}
		for i := range w {
			if w[i].ID != g[i].ID || math.Abs(w[i].Score-g[i].Score) > 1e-5 {
				t.Errorf("%q result %d = %+v, want %+v", q, i, g[i], w[i])
			}
		}
	}
}

func writeGolden(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readGolden(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create)", err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
