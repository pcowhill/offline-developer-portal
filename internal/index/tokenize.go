package index

import (
	"strings"
	"unicode"
)

// Tokenizer rules (version 1). The frontend implements exactly the same
// rules in src/search/tokenize.ts; both are tested against the shared
// vectors in internal/index/testdata/tokenizer_vectors.json.
//
//  1. A token is a maximal run of Unicode letters (\p{L}) or numbers (\p{N}).
//     Everything else (punctuation, "_", ".", whitespace) separates tokens.
//  2. Tokens are lower-cased.
//  3. Tokens longer than MaxTokenLength runes are dropped.
//  4. Stop words are removed at index time. (At query time the frontend
//     keeps them only if the query consists solely of stop words.)
const MaxTokenLength = 40

// StopWords is written into the search index so that clients do not need
// their own copy.
//
// The list is deliberately short: words such as "for", "if", "in", "is",
// "not", "and", "or", "with" and "as" are language keywords that
// developers search for, so they are kept.
var StopWords = []string{
	"a", "an", "are", "be", "by", "into", "of", "on", "such", "that", "the",
	"their", "there", "these", "they", "this", "to", "was", "were", "will",
}

var stopSet = func() map[string]bool {
	m := make(map[string]bool, len(StopWords))
	for _, w := range StopWords {
		m[w] = true
	}
	return m
}()

// IsStopWord reports whether tok is a stop word.
func IsStopWord(tok string) bool { return stopSet[tok] }

// Tokenize splits text into lower-case tokens according to the rules above,
// without removing stop words.
func Tokenize(text string) []string {
	var out []string
	start := -1
	flush := func(end int) {
		if start < 0 {
			return
		}
		tok := strings.ToLower(text[start:end])
		if n := len([]rune(tok)); n > 0 && n <= MaxTokenLength {
			out = append(out, tok)
		}
		start = -1
	}
	for i, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		flush(i)
	}
	flush(len(text))
	return out
}

// IndexTokens tokenizes text and removes stop words.
func IndexTokens(text string) []string {
	toks := Tokenize(text)
	out := toks[:0]
	for _, t := range toks {
		if !stopSet[t] {
			out = append(out, t)
		}
	}
	return out
}
