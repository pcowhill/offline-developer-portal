// Package extract turns an HTML page into a structured, readable document
// (title, headings, paragraphs, lists, tables and code blocks) and collects
// the links found on the page. It never executes JavaScript.
package extract

import (
	"bytes"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/pcowhill/offline-developer-portal/internal/index"
	"github.com/pcowhill/offline-developer-portal/internal/urlnorm"
)

// Page is the result of extracting one HTML document.
type Page struct {
	Title       string
	Description string
	Headings    []index.Heading
	Blocks      []index.Block
	// Links are absolute, normalized http(s) URLs found anywhere on the
	// page (including navigation), in document order, without duplicates.
	Links    []*url.URL
	NoIndex  bool
	NoFollow bool
}

// Options control extraction.
type Options struct {
	URLOptions urlnorm.Options
}

// skippedTags are never part of readable content.
var skippedTags = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true, atom.Template: true,
	atom.Svg: true, atom.Canvas: true, atom.Iframe: true, atom.Object: true,
	atom.Embed: true, atom.Form: true, atom.Button: true, atom.Select: true, atom.Input: true,
	atom.Textarea: true, atom.Nav: true, atom.Aside: true, atom.Footer: true, atom.Img: true,
	atom.Video: true, atom.Audio: true, atom.Picture: true, atom.Map: true, atom.Dialog: true,
	atom.Head: true, atom.Link: true, atom.Meta: true, atom.Hr: true,
}

// skippedRoles are ARIA landmarks that hold navigation chrome.
var skippedRoles = map[string]bool{
	"navigation": true, "banner": true, "contentinfo": true, "search": true,
	"complementary": true, "toolbar": true, "menu": true, "menubar": true, "dialog": true,
}

// skippedClassTokens are class or id values (matched as whole tokens) that
// almost always mark site chrome rather than content.
var skippedClassTokens = map[string]bool{
	"sidebar": true, "sphinxsidebar": true, "sphinxsidebarwrapper": true, "navbar": true,
	"nav": true, "navigation": true, "menu": true, "breadcrumb": true, "breadcrumbs": true,
	"footer": true, "related": true, "headerlink": true, "hash-link": true, "anchorjs-link": true,
	"skip-link": true, "skiplink": true, "sr-only": true, "visually-hidden": true,
	"linenos": true, "lineno": true, "copybtn": true, "copy-button": true, "toc": true,
	"table-of-contents": true, "edit-this-page": true, "theme-doc-toc-mobile": true,
	"mobile-nav": true, "site-header": true, "site-footer": true, "cookie-banner": true,
}

var inlineTags = map[atom.Atom]bool{
	atom.A: true, atom.Abbr: true, atom.B: true, atom.Bdi: true, atom.Bdo: true, atom.Cite: true,
	atom.Code: true, atom.Data: true, atom.Dfn: true, atom.Em: true, atom.I: true, atom.Kbd: true,
	atom.Mark: true, atom.Q: true, atom.S: true, atom.Samp: true, atom.Small: true, atom.Span: true,
	atom.Strong: true, atom.Sub: true, atom.Sup: true, atom.Time: true, atom.U: true, atom.Var: true,
	atom.Wbr: true, atom.Br: true, atom.Del: true, atom.Ins: true, atom.Label: true, atom.Tt: true,
	atom.Font: true, atom.Big: true, atom.Nobr: true, atom.Strike: true,
}

// Extract parses an HTML document fetched from pageURL.
func Extract(body []byte, pageURL *url.URL, opts Options) (*Page, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	e := &extractor{base: pageURL, opts: opts, anchors: map[string]bool{}, seenLinks: map[string]bool{}}
	page := &Page{}

	htmlNode := findFirst(doc, func(n *html.Node) bool { return n.DataAtom == atom.Html })
	head := findFirst(doc, func(n *html.Node) bool { return n.DataAtom == atom.Head })
	bodyNode := findFirst(doc, func(n *html.Node) bool { return n.DataAtom == atom.Body })
	if htmlNode == nil || bodyNode == nil {
		return page, nil
	}
	if head != nil {
		e.readHead(head, page)
	}
	e.collectLinks(bodyNode)
	page.Links = e.links

	root := chooseContentRoot(bodyNode)
	e.rootIsBody = root == bodyNode
	page.Blocks = e.blocks(root, 0)
	page.Headings = e.headings

	if page.Title == "" {
		for _, h := range page.Headings {
			if h.Level == 1 {
				page.Title = h.Text
				break
			}
		}
	}
	if page.Title == "" {
		page.Title = pageURL.Path
	}
	return page, nil
}

type extractor struct {
	base       *url.URL
	opts       Options
	rootIsBody bool
	headings   []index.Heading
	anchors    map[string]bool
	links      []*url.URL
	seenLinks  map[string]bool
}

func (e *extractor) readHead(head *html.Node, page *Page) {
	for n := head.FirstChild; n != nil; n = n.NextSibling {
		if n.Type != html.ElementNode {
			continue
		}
		switch n.DataAtom {
		case atom.Title:
			page.Title = collapse(textContent(n))
		case atom.Base:
			if href := attr(n, "href"); href != "" {
				if b, err := e.base.Parse(href); err == nil && (b.Scheme == "http" || b.Scheme == "https") {
					e.base = b
				}
			}
		case atom.Meta:
			name := strings.ToLower(attr(n, "name"))
			prop := strings.ToLower(attr(n, "property"))
			content := attr(n, "content")
			switch {
			case name == "robots":
				for _, d := range strings.Split(strings.ToLower(content), ",") {
					switch strings.TrimSpace(d) {
					case "noindex", "none":
						page.NoIndex = true
					}
					switch strings.TrimSpace(d) {
					case "nofollow", "none":
						page.NoFollow = true
					}
				}
			case name == "description" || (prop == "og:description" && page.Description == ""):
				page.Description = collapse(content)
			}
		}
	}
}

func (e *extractor) collectLinks(n *html.Node) {
	if n.Type == html.ElementNode && n.DataAtom == atom.A {
		if href := attr(n, "href"); href != "" {
			if u, err := urlnorm.Resolve(e.base, href, e.opts.URLOptions); err == nil {
				if s := u.String(); !e.seenLinks[s] {
					e.seenLinks[s] = true
					e.links = append(e.links, u)
				}
			}
		}
	}
	if n.Type == html.ElementNode && (n.DataAtom == atom.Script || n.DataAtom == atom.Style || n.DataAtom == atom.Template) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		e.collectLinks(c)
	}
}

// chooseContentRoot picks the element most likely to contain the main
// article: <main>/role=main, then <article>, then well-known content
// containers, falling back to <body>.
func chooseContentRoot(body *html.Node) *html.Node {
	tiers := []func(n *html.Node) bool{
		func(n *html.Node) bool { return n.DataAtom == atom.Main || attr(n, "role") == "main" },
		func(n *html.Node) bool { return n.DataAtom == atom.Article },
		func(n *html.Node) bool {
			for _, t := range classAndID(n) {
				switch t {
				case "content", "main-content", "maincontent", "docs-content", "page-content",
					"markdown-body", "rst-content", "document", "doc-content", "article":
					return true
				}
			}
			return false
		},
	}
	for _, match := range tiers {
		var best *html.Node
		bestLen := 0
		walk(body, func(n *html.Node) bool {
			if n.Type == html.ElementNode && skippedTags[n.DataAtom] {
				return false
			}
			if n.Type == html.ElementNode && match(n) {
				if l := textLength(n); l > bestLen {
					best, bestLen = n, l
				}
			}
			return true
		})
		if best != nil && bestLen >= 100 {
			return best
		}
	}
	return body
}

func (e *extractor) skip(n *html.Node) bool {
	if n.Type == html.CommentNode || n.Type == html.DoctypeNode {
		return true
	}
	if n.Type != html.ElementNode {
		return false
	}
	if skippedTags[n.DataAtom] {
		return true
	}
	if e.rootIsBody && n.DataAtom == atom.Header {
		return true
	}
	if _, ok := getAttr(n, "hidden"); ok {
		return true
	}
	if attr(n, "aria-hidden") == "true" {
		return true
	}
	if skippedRoles[strings.ToLower(attr(n, "role"))] {
		return true
	}
	style := strings.ReplaceAll(strings.ToLower(attr(n, "style")), " ", "")
	if strings.Contains(style, "display:none") || strings.Contains(style, "visibility:hidden") {
		return true
	}
	for _, t := range classAndID(n) {
		if skippedClassTokens[t] || strings.Contains(t, "breadcrumb") {
			return true
		}
	}
	return false
}

// blocks converts the children of n into blocks. Loose inline content is
// gathered into paragraphs.
func (e *extractor) blocks(n *html.Node, depth int) []index.Block {
	if depth > 60 {
		return nil
	}
	var out []index.Block
	var inline []index.Span
	flush := func() {
		if p := makeParagraph(inline); p != nil {
			out = append(out, *p)
		}
		inline = nil
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if e.skip(c) {
			continue
		}
		if c.Type == html.TextNode {
			inline = append(inline, index.Span{Text: collapseKeepEdges(c.Data)})
			continue
		}
		if c.Type != html.ElementNode {
			continue
		}
		if inlineTags[c.DataAtom] && !e.hasBlockDescendant(c) {
			inline = append(inline, e.spans(c, index.Span{})...)
			continue
		}
		flush()
		out = append(out, e.block(c, depth)...)
	}
	flush()
	return out
}

func (e *extractor) block(c *html.Node, depth int) []index.Block {
	switch c.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		return e.heading(c)
	case atom.P:
		if e.hasBlockDescendant(c) {
			return e.blocks(c, depth+1)
		}
		if p := makeParagraph(e.spans(c, index.Span{})); p != nil {
			return []index.Block{*p}
		}
		return nil
	case atom.Pre:
		text := strings.TrimRight(preText(c), "\n\r\t ")
		text = strings.TrimLeft(text, "\n\r")
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []index.Block{{Type: index.BlockCode, Text: text, Lang: codeLang(c)}}
	case atom.Ul, atom.Ol, atom.Menu:
		lst := index.Block{Type: index.BlockList, Ordered: c.DataAtom == atom.Ol}
		for li := c.FirstChild; li != nil; li = li.NextSibling {
			if e.skip(li) || li.Type != html.ElementNode {
				continue
			}
			item := e.blocks(li, depth+1)
			if len(item) > 0 {
				lst.Items = append(lst.Items, item)
			}
		}
		if len(lst.Items) == 0 {
			return nil
		}
		return []index.Block{lst}
	case atom.Dl:
		var out []index.Block
		for d := c.FirstChild; d != nil; d = d.NextSibling {
			if e.skip(d) || d.Type != html.ElementNode {
				continue
			}
			switch d.DataAtom {
			case atom.Dt:
				if e.hasBlockDescendant(d) {
					out = append(out, e.blocks(d, depth+1)...)
				} else if p := makeParagraph(e.spans(d, index.Span{Bold: true})); p != nil {
					out = append(out, *p)
				}
			case atom.Dd:
				if inner := e.blocks(d, depth+1); len(inner) > 0 {
					out = append(out, index.Block{Type: index.BlockQuote, Children: inner})
				}
			default:
				out = append(out, e.blocks(d, depth+1)...)
			}
		}
		return out
	case atom.Blockquote:
		inner := e.blocks(c, depth+1)
		if len(inner) == 0 {
			return nil
		}
		return []index.Block{{Type: index.BlockQuote, Children: inner}}
	case atom.Table:
		if containsTag(c, atom.Pre) || containsTag(c, atom.Table, true) {
			return e.blocks(c, depth+1)
		}
		return e.table(c)
	default:
		return e.blocks(c, depth+1)
	}
}

func (e *extractor) heading(c *html.Node) []index.Block {
	spans := e.spans(c, index.Span{})
	// Headings carry no links or code styling in the output.
	text := strings.TrimSpace(collapse(index.SpansText(spans)))
	text = strings.TrimRightFunc(text, func(r rune) bool {
		return r == '¶' || r == '#' || r == '§' || unicode.IsSpace(r) || r == ''
	})
	if text == "" {
		return nil
	}
	level := int(c.Data[1] - '0')
	anchor := e.anchorFor(c, text)
	e.headings = append(e.headings, index.Heading{Level: level, Text: text, Anchor: anchor})
	return []index.Block{{Type: index.BlockHeading, Level: level, Anchor: anchor, Spans: []index.Span{{Text: text}}}}
}

var slugStrip = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func (e *extractor) anchorFor(h *html.Node, text string) string {
	candidates := []string{attr(h, "id")}
	walk(h, func(n *html.Node) bool {
		if n != h && n.Type == html.ElementNode {
			candidates = append(candidates, attr(n, "id"), attr(n, "name"))
		}
		return true
	})
	if p := h.Parent; p != nil && p.DataAtom == atom.Section {
		candidates = append(candidates, attr(p, "id"))
	}
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c != "" && !e.anchors[c] {
			e.anchors[c] = true
			return c
		}
	}
	slug := strings.Trim(slugStrip.ReplaceAllString(strings.ToLower(text), "-"), "-")
	if slug == "" {
		slug = "section"
	}
	a := slug
	for i := 2; e.anchors[a]; i++ {
		a = slug + "-" + strconv.Itoa(i)
	}
	e.anchors[a] = true
	return a
}

// spans flattens inline content into styled spans.
func (e *extractor) spans(n *html.Node, style index.Span) []index.Span {
	var out []index.Span
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if e.skip(c) {
			continue
		}
		switch c.Type {
		case html.TextNode:
			s := style
			s.Text = collapseKeepEdges(c.Data)
			out = append(out, s)
		case html.ElementNode:
			s := style
			switch c.DataAtom {
			case atom.Br:
				s.Text = " "
				out = append(out, s)
				continue
			case atom.Code, atom.Kbd, atom.Samp, atom.Tt, atom.Var:
				s.Code = true
			case atom.Strong, atom.B:
				s.Bold = true
			case atom.Em, atom.I, atom.Cite, atom.Dfn:
				s.Italic = true
			case atom.A:
				if href := attr(c, "href"); href != "" {
					s.Href = e.resolveHref(href)
				}
			}
			out = append(out, e.spans(c, s)...)
		}
	}
	return out
}

// resolveHref turns a link into an absolute, normalized URL, keeping the
// fragment so in-page targets survive. Non-http links are dropped.
func (e *extractor) resolveHref(href string) string {
	href = strings.TrimSpace(href)
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	abs := e.base.ResolveReference(ref)
	u, err := urlnorm.Normalize(abs, e.opts.URLOptions)
	if err != nil {
		return ""
	}
	s := u.String()
	if abs.Fragment != "" {
		s += "#" + abs.EscapedFragment()
	}
	return s
}

func (e *extractor) table(t *html.Node) []index.Block {
	var rows [][]string
	header := false
	first := true
	walk(t, func(n *html.Node) bool {
		if n != t && n.Type == html.ElementNode && e.skip(n) {
			return false
		}
		if n.Type == html.ElementNode && n.DataAtom == atom.Tr {
			var cells []string
			allTH := true
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.DataAtom == atom.Td || c.DataAtom == atom.Th) {
					cells = append(cells, strings.TrimSpace(collapse(index.SpansText(e.spans(c, index.Span{})))))
					if c.DataAtom != atom.Th {
						allTH = false
					}
				}
			}
			if len(cells) > 0 {
				if first {
					header = allTH || (n.Parent != nil && n.Parent.DataAtom == atom.Thead)
					first = false
				}
				rows = append(rows, cells)
			}
			return false
		}
		return true
	})
	if len(rows) == 0 {
		return nil
	}
	return []index.Block{{Type: index.BlockTable, Rows: rows, Header: header}}
}

func (e *extractor) hasBlockDescendant(n *html.Node) bool {
	found := false
	walk(n, func(c *html.Node) bool {
		if found {
			return false
		}
		if c != n && c.Type == html.ElementNode {
			if e.skip(c) {
				return false
			}
			if !inlineTags[c.DataAtom] {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func makeParagraph(spans []index.Span) *index.Block {
	var merged []index.Span
	for _, s := range spans {
		if s.Text == "" {
			continue
		}
		if l := len(merged); l > 0 {
			prev := &merged[l-1]
			if prev.Code == s.Code && prev.Bold == s.Bold && prev.Italic == s.Italic && prev.Href == s.Href {
				prev.Text += s.Text
				continue
			}
			if strings.HasSuffix(prev.Text, " ") && strings.HasPrefix(s.Text, " ") {
				s.Text = strings.TrimLeft(s.Text, " ")
				if s.Text == "" {
					continue
				}
			}
		}
		merged = append(merged, s)
	}
	for i := range merged {
		merged[i].Text = strings.ReplaceAll(merged[i].Text, "  ", " ")
	}
	for len(merged) > 0 {
		merged[0].Text = strings.TrimLeft(merged[0].Text, " ")
		if merged[0].Text != "" {
			break
		}
		merged = merged[1:]
	}
	for len(merged) > 0 {
		l := len(merged) - 1
		merged[l].Text = strings.TrimRight(merged[l].Text, " ")
		if merged[l].Text != "" {
			break
		}
		merged = merged[:l]
	}
	if len(merged) == 0 {
		return nil
	}
	return &index.Block{Type: index.BlockParagraph, Spans: merged}
}

var langClass = regexp.MustCompile(`^(?:language|lang|highlight|brush)-([A-Za-z0-9_+#.-]+)$`)

func codeLang(pre *html.Node) string {
	nodes := []*html.Node{pre}
	if code := findFirst(pre, func(n *html.Node) bool { return n.DataAtom == atom.Code }); code != nil {
		nodes = append(nodes, code)
	}
	for p, i := pre.Parent, 0; p != nil && i < 3; p, i = p.Parent, i+1 {
		nodes = append(nodes, p)
	}
	for _, n := range nodes {
		for _, cls := range strings.Fields(attr(n, "class")) {
			if m := langClass.FindStringSubmatch(cls); m != nil {
				l := strings.ToLower(m[1])
				if l == "default" || l == "none" || l == "text" || l == "plaintext" {
					return ""
				}
				return l
			}
		}
		if l := attr(n, "data-lang"); l != "" {
			return strings.ToLower(l)
		}
	}
	return ""
}

// preText returns the text of a <pre> element with whitespace intact,
// skipping line-number gutters and copy buttons.
func preText(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.TextNode:
				b.WriteString(c.Data)
			case html.ElementNode:
				if c.DataAtom == atom.Br {
					b.WriteByte('\n')
					continue
				}
				if c.DataAtom == atom.Button || c.DataAtom == atom.Script || c.DataAtom == atom.Style {
					continue
				}
				skip := false
				for _, t := range classAndID(c) {
					if t == "linenos" || t == "lineno" || t == "line-numbers-rows" || t == "copybtn" || t == "copy-button" {
						skip = true
					}
				}
				if attr(c, "aria-hidden") == "true" {
					skip = true
				}
				if !skip {
					rec(c)
				}
			}
		}
	}
	rec(n)
	return strings.ReplaceAll(b.String(), "\r\n", "\n")
}

// ---- small DOM helpers ----

func attr(n *html.Node, key string) string {
	v, _ := getAttr(n, key)
	return v
}

func getAttr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Namespace == "" && strings.EqualFold(a.Key, key) {
			return a.Val, true
		}
	}
	return "", false
}

func classAndID(n *html.Node) []string {
	toks := strings.Fields(strings.ToLower(attr(n, "class")))
	if id := strings.ToLower(strings.TrimSpace(attr(n, "id"))); id != "" {
		toks = append(toks, id)
	}
	return toks
}

// walk visits n and its descendants depth-first; returning false from fn
// skips the node's children.
func walk(n *html.Node, fn func(*html.Node) bool) {
	if !fn(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, fn)
	}
}

func findFirst(n *html.Node, pred func(*html.Node) bool) *html.Node {
	var found *html.Node
	walk(n, func(c *html.Node) bool {
		if found != nil {
			return false
		}
		if c.Type == html.ElementNode && pred(c) {
			found = c
			return false
		}
		return true
	})
	return found
}

func containsTag(n *html.Node, a atom.Atom, excludeSelf ...bool) bool {
	return findFirst(n, func(c *html.Node) bool {
		return c.DataAtom == a && !(len(excludeSelf) > 0 && excludeSelf[0] && c == n)
	}) != nil
}

func textContent(n *html.Node) string {
	var b strings.Builder
	walk(n, func(c *html.Node) bool {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
		return true
	})
	return b.String()
}

func textLength(n *html.Node) int {
	l := 0
	walk(n, func(c *html.Node) bool {
		if c.Type == html.ElementNode && (c.DataAtom == atom.Script || c.DataAtom == atom.Style || c.DataAtom == atom.Nav) {
			return false
		}
		if c.Type == html.TextNode {
			for _, r := range c.Data {
				if !unicode.IsSpace(r) {
					l++
				}
			}
		}
		return true
	})
	return l
}

func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// collapseKeepEdges collapses internal whitespace but keeps a single
// leading/trailing space if the original had one, so adjacent inline runs
// do not merge words.
func collapseKeepEdges(s string) string {
	if s == "" {
		return ""
	}
	inner := collapse(s)
	if inner == "" {
		return " "
	}
	if unicode.IsSpace(rune(s[0])) {
		inner = " " + inner
	}
	if unicode.IsSpace(rune(s[len(s)-1])) {
		inner += " "
	}
	return inner
}
