package index

import (
	"math"
	"sort"
	"strings"
)

// The ranking algorithm below is mirrored by src/search/engine.ts. Keep the
// two in sync.
const (
	prefixFactor        = 0.5
	maxPrefixExpansions = 30
	minPrefixLength     = 2
)

// SearchOptions restrict a search.
type SearchOptions struct {
	// Sources limits results to these source ids (empty means all).
	Sources []string
	// Limit caps the number of results (0 means no limit).
	Limit int
}

// Result is one ranked hit.
type Result struct {
	Doc   IndexedDoc
	Score float64
}

// QueryTerms tokenizes a query: stop words are removed unless the query
// consists only of stop words; duplicates are removed.
func QueryTerms(query string) []string {
	toks := Tokenize(query)
	var kept []string
	seen := map[string]bool{}
	for _, t := range toks {
		if !IsStopWord(t) && !seen[t] {
			seen[t] = true
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		for _, t := range toks {
			if !seen[t] {
				seen[t] = true
				kept = append(kept, t)
			}
		}
	}
	return kept
}

// Search ranks documents for query using BM25 over weighted fields, with
// prefix expansion and a coordination factor that favours documents
// matching every query term.
func (idx *SearchIndex) Search(query string, opts SearchOptions) []Result {
	terms := QueryTerms(query)
	if len(terms) == 0 || len(idx.Docs) == 0 {
		return nil
	}
	allowed := map[string]bool{}
	for _, s := range opts.Sources {
		allowed[s] = true
	}
	sorted := idx.sortedTerms()
	n := float64(len(idx.Docs))
	k1, b := idx.BM25.K1, idx.BM25.B
	avg := idx.AvgDocLength
	if avg <= 0 {
		avg = 1
	}

	total := map[int]float64{}
	matched := map[int]int{}
	for _, qt := range terms {
		type cand struct {
			term   string
			factor float64
		}
		cands := []cand{}
		if _, ok := idx.Terms[qt]; ok {
			cands = append(cands, cand{qt, 1})
		}
		if len([]rune(qt)) >= minPrefixLength {
			var exp []string
			i := sort.SearchStrings(sorted, qt)
			for ; i < len(sorted) && strings.HasPrefix(sorted[i], qt); i++ {
				if sorted[i] != qt {
					exp = append(exp, sorted[i])
				}
			}
			// Prefer the most common expansions.
			sort.SliceStable(exp, func(a, c int) bool {
				return len(idx.Terms[exp[a]]) > len(idx.Terms[exp[c]])
			})
			if len(exp) > maxPrefixExpansions {
				exp = exp[:maxPrefixExpansions]
			}
			for _, e := range exp {
				cands = append(cands, cand{e, prefixFactor})
			}
		}
		best := map[int]float64{}
		for _, c := range cands {
			postings := idx.Terms[c.term]
			df := float64(len(postings) / 2)
			idf := math.Log(1 + (n-df+0.5)/(df+0.5))
			for p := 0; p+1 < len(postings); p += 2 {
				d, tf := postings[p], float64(postings[p+1])
				dl := float64(idx.Docs[d].Length)
				s := c.factor * idf * tf * (k1 + 1) / (tf + k1*(1-b+b*dl/avg))
				if s > best[d] {
					best[d] = s
				}
			}
		}
		for d, s := range best {
			total[d] += s
			matched[d]++
		}
	}

	phrase := strings.ToLower(strings.Join(strings.Fields(query), " "))
	results := make([]Result, 0, len(total))
	for d, s := range total {
		doc := idx.Docs[d]
		if len(allowed) > 0 && !allowed[doc.SourceID] {
			continue
		}
		coord := float64(matched[d]) / float64(len(terms))
		score := s * coord * coord
		title := strings.ToLower(doc.Title)
		if phrase != "" && title == phrase {
			score *= 2
		} else if len(terms) > 1 && strings.Contains(title, phrase) {
			score *= 1.5
		}
		results = append(results, Result{Doc: doc, Score: score})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Doc.ID < results[j].Doc.ID
	})
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}
	return results
}

func (idx *SearchIndex) sortedTerms() []string {
	if idx.sorted == nil {
		idx.sorted = make([]string, 0, len(idx.Terms))
		for t := range idx.Terms {
			idx.sorted = append(idx.sorted, t)
		}
		sort.Strings(idx.sorted)
	}
	return idx.sorted
}
