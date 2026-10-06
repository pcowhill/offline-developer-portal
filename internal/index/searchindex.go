package index

import (
	"sort"
)

// Field weights applied to term frequencies. A term in the title counts as
// much as six occurrences in the body.
const (
	WeightTitle   = 6
	WeightHeading = 3
	WeightBody    = 1
	BM25K1        = 1.2
	BM25B         = 0.75
)

// SearchIndex is the pre-computed inverted index written to
// search-index.json and loaded by the portal (and by `offline-docs search`).
type SearchIndex struct {
	Format        string         `json:"format"`
	FormatVersion int            `json:"format_version"`
	Tokenizer     TokenizerInfo  `json:"tokenizer"`
	Weights       map[string]int `json:"weights"`
	BM25          BM25Params     `json:"bm25"`
	AvgDocLength  float64        `json:"avg_doc_length"`
	Docs          []IndexedDoc   `json:"docs"`
	// Terms maps each term to a flat posting list
	// [docIndex0, weightedTF0, docIndex1, weightedTF1, ...] sorted by doc index.
	Terms map[string][]int `json:"terms"`

	sorted []string // lazily built sorted term list
}

// TokenizerInfo describes how text was tokenized.
type TokenizerInfo struct {
	Version        int      `json:"version"`
	MaxTokenLength int      `json:"max_token_length"`
	StopWords      []string `json:"stop_words"`
}

// BM25Params are the ranking parameters used by clients.
type BM25Params struct {
	K1 float64 `json:"k1"`
	B  float64 `json:"b"`
}

// IndexedDoc is the per-document metadata needed to render a result list
// without loading the full document.
type IndexedDoc struct {
	ID       string `json:"id"`
	SourceID string `json:"source_id"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Length   int    `json:"length"`
	Summary  string `json:"summary"`
}

// BuildSearchIndex computes the inverted index for docs. Documents are
// ordered by source then title so output is deterministic.
func BuildSearchIndex(docs []*Document) *SearchIndex {
	ordered := append([]*Document(nil), docs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].SourceID != ordered[j].SourceID {
			return ordered[i].SourceID < ordered[j].SourceID
		}
		if ordered[i].Title != ordered[j].Title {
			return ordered[i].Title < ordered[j].Title
		}
		return ordered[i].URL < ordered[j].URL
	})

	idx := &SearchIndex{
		Format:        SearchIndexFormat,
		FormatVersion: FormatVersion,
		Tokenizer: TokenizerInfo{
			Version:        TokenizerVersion,
			MaxTokenLength: MaxTokenLength,
			StopWords:      StopWords,
		},
		Weights: map[string]int{"title": WeightTitle, "heading": WeightHeading, "body": WeightBody},
		BM25:    BM25Params{K1: BM25K1, B: BM25B},
		Docs:    make([]IndexedDoc, 0, len(ordered)),
		Terms:   map[string][]int{},
	}

	total := 0
	for i, d := range ordered {
		tf := map[string]int{}
		for _, t := range IndexTokens(d.Title) {
			tf[t] += WeightTitle
		}
		headings, body := d.fieldText()
		for _, h := range headings {
			for _, t := range IndexTokens(h) {
				tf[t] += WeightHeading
			}
		}
		for _, b := range body {
			for _, t := range IndexTokens(b) {
				tf[t] += WeightBody
			}
		}
		length := 0
		for _, n := range tf {
			length += n
		}
		total += length
		idx.Docs = append(idx.Docs, IndexedDoc{
			ID:       d.ID,
			SourceID: d.SourceID,
			Title:    d.Title,
			URL:      d.URL,
			Length:   length,
			Summary:  d.Summary(),
		})
		for t, n := range tf {
			idx.Terms[t] = append(idx.Terms[t], i, n)
		}
	}
	if len(ordered) > 0 {
		idx.AvgDocLength = float64(total) / float64(len(ordered))
	}
	return idx
}
