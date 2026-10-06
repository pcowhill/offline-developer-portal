// Package index defines the offline corpus format (documents, manifest and
// search index), builds it from crawled documents, writes it to disk and
// reads it back.
//
// The format is plain JSON so that the static portal, a future CLI, or a
// future MCP server can all consume the same files without a running
// backend. See docs/corpus-format.md for the full description.
package index

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// Format identifiers written into every corpus file.
const (
	CorpusFormat      = "offline-developer-portal-corpus"
	SearchIndexFormat = "offline-developer-portal-search-index"
	FormatVersion     = 1
	TokenizerVersion  = 1
	ManifestFile      = "manifest.json"
	SearchIndexFile   = "search-index.json"
	DocumentsDir      = "docs"
	summaryMaxRunes   = 240
)

// Document is one indexed page, stored as docs/<id[:2]>/<id>.json.
type Document struct {
	ID          string    `json:"id"`
	SourceID    string    `json:"source_id"`
	SourceName  string    `json:"source_name"`
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	IndexedAt   time.Time `json:"indexed_at"`
	Headings    []Heading `json:"headings"`
	Blocks      []Block   `json:"blocks"`
}

// Heading is an entry of the document's table of contents.
type Heading struct {
	Level  int    `json:"level"`
	Text   string `json:"text"`
	Anchor string `json:"anchor"`
}

// Block types.
const (
	BlockHeading   = "heading"
	BlockParagraph = "paragraph"
	BlockCode      = "code"
	BlockList      = "list"
	BlockQuote     = "quote"
	BlockTable     = "table"
)

// Block is one structural element of a document. Which fields are used
// depends on Type:
//
//	heading:   Level, Anchor, Spans
//	paragraph: Spans
//	code:      Text, Lang
//	list:      Ordered, Items (each item is a list of blocks)
//	quote:     Children
//	table:     Rows (cells of plain text); Header marks the first row as a header
type Block struct {
	Type     string     `json:"type"`
	Level    int        `json:"level,omitempty"`
	Anchor   string     `json:"anchor,omitempty"`
	Spans    []Span     `json:"spans,omitempty"`
	Text     string     `json:"text,omitempty"`
	Lang     string     `json:"lang,omitempty"`
	Ordered  bool       `json:"ordered,omitempty"`
	Items    [][]Block  `json:"items,omitempty"`
	Children []Block    `json:"children,omitempty"`
	Rows     [][]string `json:"rows,omitempty"`
	Header   bool       `json:"header,omitempty"`
}

// Span is a run of inline text with optional formatting.
type Span struct {
	Text   string `json:"t"`
	Code   bool   `json:"c,omitempty"`
	Bold   bool   `json:"b,omitempty"`
	Italic bool   `json:"i,omitempty"`
	Href   string `json:"h,omitempty"`
}

// DocumentID returns the stable identifier of a URL within a source.
func DocumentID(sourceID, url string) string {
	sum := sha256.Sum256([]byte(sourceID + "\x00" + url))
	return hex.EncodeToString(sum[:8])
}

// SpansText concatenates the text of spans.
func SpansText(spans []Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

// fieldText splits a document into the text of its weighted fields.
func (d *Document) fieldText() (headings, body []string) {
	var walk func(blocks []Block)
	walk = func(blocks []Block) {
		for _, b := range blocks {
			switch b.Type {
			case BlockHeading:
				headings = append(headings, SpansText(b.Spans))
			case BlockParagraph:
				body = append(body, SpansText(b.Spans))
			case BlockCode:
				body = append(body, b.Text)
			case BlockList:
				for _, item := range b.Items {
					walk(item)
				}
			case BlockQuote:
				walk(b.Children)
			case BlockTable:
				for _, row := range b.Rows {
					body = append(body, strings.Join(row, " "))
				}
			}
		}
	}
	walk(d.Blocks)
	return headings, body
}

// PlainText returns the readable prose of the document (excluding code),
// used for summaries.
func (d *Document) PlainText() string {
	var parts []string
	var walk func(blocks []Block)
	walk = func(blocks []Block) {
		for _, b := range blocks {
			switch b.Type {
			case BlockParagraph:
				parts = append(parts, SpansText(b.Spans))
			case BlockList:
				for _, item := range b.Items {
					walk(item)
				}
			case BlockQuote:
				walk(b.Children)
			}
		}
	}
	walk(d.Blocks)
	return strings.Join(parts, " ")
}

// Summary returns a short description used in result lists before the full
// document has been loaded.
func (d *Document) Summary() string {
	s := strings.TrimSpace(d.Description)
	if s == "" {
		s = d.PlainText()
	}
	return truncateRunes(strings.Join(strings.Fields(s), " "), summaryMaxRunes)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n])
	if i := strings.LastIndexByte(cut, ' '); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:") + "…"
}
