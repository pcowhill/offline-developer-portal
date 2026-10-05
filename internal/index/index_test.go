package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type vectors struct {
	Cases []struct {
		Input  string   `json:"input"`
		Tokens []string `json:"tokens"`
	} `json:"cases"`
	QueryCases []struct {
		Query string   `json:"query"`
		Terms []string `json:"terms"`
	} `json:"query_cases"`
}

func TestTokenizerVectors(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "tokenizer_vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	for _, c := range v.Cases {
		got := Tokenize(c.Input)
		if len(got) == 0 && len(c.Tokens) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.Tokens) {
			t.Errorf("Tokenize(%q) = %q, want %q", c.Input, got, c.Tokens)
		}
	}
	for _, c := range v.QueryCases {
		if got := QueryTerms(c.Query); !reflect.DeepEqual(got, c.Terms) {
			t.Errorf("QueryTerms(%q) = %q, want %q", c.Query, got, c.Terms)
		}
	}
}

func para(text string) Block { return Block{Type: BlockParagraph, Spans: []Span{{Text: text}}} }

func sampleDocs() []*Document {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	mk := func(src, url, title string, blocks ...Block) *Document {
		return &Document{ID: DocumentID(src, url), SourceID: src, SourceName: strings.ToUpper(src), URL: url, Title: title, IndexedAt: at, Headings: []Heading{}, Blocks: blocks}
	}
	return []*Document{
		mk("alpha", "https://a.example/install", "Installation guide",
			Block{Type: BlockHeading, Level: 1, Anchor: "install", Spans: []Span{{Text: "Installing the tool"}}},
			para("Download the archive and run the installer."),
			Block{Type: BlockCode, Text: "tool --install --prefix /opt"}),
		mk("alpha", "https://a.example/config", "Configuration reference",
			para("Every configuration option is described here. Options can also be set with environment variables."),
			Block{Type: BlockList, Items: [][]Block{{para("timeout: request timeout")}, {para("retries: retry count")}}}),
		mk("beta", "https://b.example/loops", "Loops",
			para("A for loop repeats a block of code. The while loop repeats while a condition holds."),
			Block{Type: BlockTable, Rows: [][]string{{"Keyword", "Meaning"}, {"break", "leave the loop"}}, Header: true}),
	}
}

func TestBuildSearchIndexFormat(t *testing.T) {
	si := BuildSearchIndex(sampleDocs())
	if si.Format != SearchIndexFormat || si.FormatVersion != FormatVersion {
		t.Fatalf("bad header: %s v%d", si.Format, si.FormatVersion)
	}
	if len(si.Docs) != 3 {
		t.Fatalf("docs = %d", len(si.Docs))
	}
	// Deterministic order: by source, then title.
	gotOrder := []string{si.Docs[0].Title, si.Docs[1].Title, si.Docs[2].Title}
	if !reflect.DeepEqual(gotOrder, []string{"Configuration reference", "Installation guide", "Loops"}) {
		t.Errorf("order = %v", gotOrder)
	}
	// Title terms are weighted.
	p := si.Terms["installation"]
	if len(p) != 2 || p[0] != 1 || p[1] != WeightTitle {
		t.Errorf("postings(installation) = %v", p)
	}
	// Heading + body + code all indexed; stop words not.
	for _, term := range []string{"installing", "archive", "prefix", "timeout", "break", "while", "for"} {
		if _, ok := si.Terms[term]; !ok {
			t.Errorf("term %q missing", term)
		}
	}
	for _, sw := range []string{"the", "a", "of"} {
		if _, ok := si.Terms[sw]; ok {
			t.Errorf("stop word %q should not be indexed", sw)
		}
	}
	if si.AvgDocLength <= 0 {
		t.Error("avg doc length not computed")
	}
	// Postings are sorted by doc index.
	for term, p := range si.Terms {
		for i := 2; i < len(p); i += 2 {
			if p[i] <= p[i-2] {
				t.Fatalf("postings for %q not sorted: %v", term, p)
			}
		}
	}
	// Byte-identical output for identical input.
	a, _ := json.Marshal(BuildSearchIndex(sampleDocs()))
	b, _ := json.Marshal(BuildSearchIndex(sampleDocs()))
	if string(a) != string(b) {
		t.Error("index output is not deterministic")
	}
}

func TestSearchRanking(t *testing.T) {
	si := BuildSearchIndex(sampleDocs())
	res := si.Search("install", SearchOptions{})
	if len(res) == 0 || res[0].Doc.Title != "Installation guide" {
		t.Fatalf("install -> %v", res)
	}
	// Prefix expansion: "config" finds "configuration".
	res = si.Search("config", SearchOptions{})
	if len(res) == 0 || res[0].Doc.Title != "Configuration reference" {
		t.Fatalf("config -> %v", res)
	}
	// Documents matching all terms rank first.
	res = si.Search("while loop", SearchOptions{})
	if len(res) == 0 || res[0].Doc.Title != "Loops" {
		t.Fatalf("while loop -> %v", res)
	}
	// Source filter.
	res = si.Search("loop", SearchOptions{Sources: []string{"alpha"}})
	for _, r := range res {
		if r.Doc.SourceID != "alpha" {
			t.Fatalf("source filter leaked %v", r.Doc)
		}
	}
	if got := si.Search("zzzznotaword", SearchOptions{}); len(got) != 0 {
		t.Fatalf("expected no results, got %v", got)
	}
	if got := si.Search("   ", SearchOptions{}); got != nil {
		t.Fatalf("empty query should return nil")
	}
	if got := si.Search("o", SearchOptions{Limit: 1}); len(got) > 1 {
		t.Fatalf("limit ignored")
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "corpus")
	docs := sampleDocs()
	m := &Manifest{Generator: "test", GeneratedAt: time.Unix(0, 0).UTC(), Sources: []SourceManifest{{ID: "beta", Name: "Beta"}, {ID: "alpha", Name: "Alpha"}}}
	if err := Write(dir, m, docs); err != nil {
		t.Fatal(err)
	}
	m2, err := ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m2.DocumentCount != 3 || m2.Format != CorpusFormat || m2.SearchIndex != SearchIndexFile || m2.Sources[0].ID != "alpha" {
		t.Fatalf("manifest = %+v", m2)
	}
	si, err := ReadSearchIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(si.Docs) != 3 {
		t.Fatal("search index docs")
	}
	d, err := ReadDocument(dir, docs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, docs[0]) {
		t.Errorf("document round trip mismatch:\n%+v\n%+v", d, docs[0])
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", docs[0].ID[:2], docs[0].ID+".json")); err != nil {
		t.Errorf("document path layout: %v", err)
	}
	alpha, err := ReadDocumentsForSources(dir, map[string]bool{"alpha": true})
	if err != nil || len(alpha) != 2 {
		t.Fatalf("ReadDocumentsForSources = %d, %v", len(alpha), err)
	}
}

func TestReplaceKeepsOldCorpusUntilSwap(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "corpus")
	tmp := filepath.Join(root, "corpus.tmp")
	os.MkdirAll(dst, 0o755)
	os.WriteFile(filepath.Join(dst, "old"), []byte("old"), 0o644)
	os.MkdirAll(tmp, 0o755)
	os.WriteFile(filepath.Join(tmp, "new"), []byte("new"), 0o644)
	if err := Replace(tmp, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "new")); err != nil {
		t.Error("new corpus not in place")
	}
	if _, err := os.Stat(filepath.Join(dst, "old")); err == nil {
		t.Error("old corpus still present")
	}
	if _, err := os.Stat(dst + ".previous"); err == nil {
		t.Error("backup not cleaned up")
	}
}

func TestSummaryAndDocumentID(t *testing.T) {
	d := &Document{Blocks: []Block{para(strings.Repeat("word ", 100))}}
	s := d.Summary()
	if len([]rune(s)) > summaryMaxRunes+1 || !strings.HasSuffix(s, "…") {
		t.Errorf("summary = %q", s)
	}
	d.Description = "Short description."
	if d.Summary() != "Short description." {
		t.Error("description should be preferred")
	}
	id := DocumentID("src", "https://e.com/")
	if len(id) != 16 || id != DocumentID("src", "https://e.com/") || id == DocumentID("other", "https://e.com/") {
		t.Errorf("bad id %q", id)
	}
}
