package extract

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pcowhill/offline-developer-portal/internal/index"
)

func load(t *testing.T) *Page {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "page.html"))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse("https://docs.example.com/v2/guide/widgets.html")
	p, err := Extract(body, u, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func allText(blocks []index.Block) string {
	b, _ := json.Marshal(blocks)
	return string(b)
}

func TestTitleAndMeta(t *testing.T) {
	p := load(t)
	if p.Title != "Widgets Guide — Example Docs" {
		t.Errorf("title = %q", p.Title)
	}
	if p.Description != "How to build widgets." {
		t.Errorf("description = %q", p.Description)
	}
	if p.NoIndex || p.NoFollow {
		t.Error("robots flags should be false")
	}
}

func TestHeadingsAndAnchors(t *testing.T) {
	p := load(t)
	want := []index.Heading{
		{Level: 1, Text: "Widgets Guide", Anchor: "widgets"},
		{Level: 2, Text: "Installing", Anchor: "install-section"},
		{Level: 2, Text: "Options", Anchor: "options"},
		{Level: 2, Text: "Options", Anchor: "options-2"},
	}
	if len(p.Headings) != len(want) {
		t.Fatalf("headings = %+v", p.Headings)
	}
	for i := range want {
		if p.Headings[i] != want[i] {
			t.Errorf("heading %d = %+v, want %+v", i, p.Headings[i], want[i])
		}
	}
}

func TestChromeAndHiddenContentRemoved(t *testing.T) {
	text := allText(load(t).Blocks)
	for _, bad := range []string{"should never be indexed", "color: red", "Table of contents junk", "Copyright footer", "Hidden text", "Also hidden", "Enable JS", "Elsewhere", "¶", `"Copy"`} {
		if strings.Contains(text, bad) {
			t.Errorf("extracted content contains %q", bad)
		}
	}
	for _, good := range []string{"reusable", "thread-safe", "Quoted wisdom", "How long to wait"} {
		if !strings.Contains(text, good) {
			t.Errorf("extracted content is missing %q", good)
		}
	}
}

func TestInlineFormattingAndLinks(t *testing.T) {
	p := load(t)
	var para *index.Block
	for i := range p.Blocks {
		if p.Blocks[i].Type == index.BlockParagraph && strings.HasPrefix(index.SpansText(p.Blocks[i].Spans), "Widgets are") {
			para = &p.Blocks[i]
		}
	}
	if para == nil {
		t.Fatalf("paragraph not found in %s", allText(p.Blocks))
	}
	got := index.SpansText(para.Spans)
	if got != "Widgets are small, reusable components. Call widget.New() to create one, or read the gadget docs." {
		t.Errorf("paragraph text = %q", got)
	}
	var sawBold, sawCode, sawLink bool
	for _, s := range para.Spans {
		sawBold = sawBold || (s.Bold && s.Text == "small")
		sawCode = sawCode || (s.Code && s.Text == "widget.New()")
		// Relative links resolve against <base href> and keep the fragment.
		sawLink = sawLink || (s.Href == "https://docs.example.com/v2/guide/gadgets.html#make" && s.Text == "gadget docs")
	}
	if !sawBold || !sawCode || !sawLink {
		t.Errorf("formatting lost: %+v", para.Spans)
	}
}

func TestCodeBlocks(t *testing.T) {
	p := load(t)
	var codes []index.Block
	var walk func([]index.Block)
	walk = func(bs []index.Block) {
		for _, b := range bs {
			if b.Type == index.BlockCode {
				codes = append(codes, b)
			}
			for _, it := range b.Items {
				walk(it)
			}
			walk(b.Children)
		}
	}
	walk(p.Blocks)
	if len(codes) != 2 {
		t.Fatalf("code blocks = %+v", codes)
	}
	if codes[0].Text != "$ pip install widgets\n$ widgets --version" || codes[0].Lang != "shell" {
		t.Errorf("code[0] = %q (%s); line numbers must be stripped and whitespace kept", codes[0].Text, codes[0].Lang)
	}
	if codes[1].Text != `fmt.Println("hi")` || codes[1].Lang != "go" {
		t.Errorf("code[1] = %+v", codes[1])
	}
}

func TestListsTablesAndDefinitions(t *testing.T) {
	p := load(t)
	var list, table *index.Block
	for i := range p.Blocks {
		switch p.Blocks[i].Type {
		case index.BlockList:
			if list == nil {
				list = &p.Blocks[i]
			}
		case index.BlockTable:
			table = &p.Blocks[i]
		}
	}
	if list == nil || len(list.Items) != 2 {
		t.Fatalf("list = %+v", list)
	}
	second := list.Items[1]
	if len(second) != 3 || second[0].Type != index.BlockParagraph || second[1].Type != index.BlockCode || second[2].Type != index.BlockList {
		t.Errorf("second item = %s", allText(second))
	}
	if table == nil || !table.Header || len(table.Rows) != 3 || table.Rows[0][0] != "Name" || table.Rows[2][1] != "blue" {
		t.Errorf("table = %+v", table)
	}
}

func TestLinksCollectedFromWholePage(t *testing.T) {
	p := load(t)
	var got []string
	for _, l := range p.Links {
		got = append(got, l.String())
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{
		"https://docs.example.com/",
		"https://docs.example.com/v2/guide/widgets.html",
		"https://docs.example.com/v2/guide/gadgets.html",
		"https://other.example.org/",
	} {
		if !strings.Contains(joined+"\n", want+"\n") {
			t.Errorf("link %s missing from %v", want, got)
		}
	}
	if strings.Contains(joined, "mailto") || strings.Contains(joined, "javascript") || strings.Contains(joined, "#") {
		t.Errorf("unexpected links: %v", got)
	}
	// Duplicates removed: gadgets.html appears twice with different fragments.
	count := strings.Count(joined, "gadgets.html")
	if count != 1 {
		t.Errorf("gadgets.html collected %d times", count)
	}
}

func TestMetaRobotsAndFallbacks(t *testing.T) {
	u, _ := url.Parse("https://e.com/x/page")
	p, err := Extract([]byte(`<html><head><meta name="robots" content="NOINDEX, nofollow"></head><body><h1>Only heading</h1><p>Text</p></body></html>`), u, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !p.NoIndex || !p.NoFollow {
		t.Error("meta robots not detected")
	}
	if p.Title != "Only heading" {
		t.Errorf("title fallback = %q", p.Title)
	}
	p, _ = Extract([]byte(`<p>bare</p>`), u, Options{})
	if p.Title != "/x/page" || len(p.Blocks) != 1 {
		t.Errorf("bare document: %+v", p)
	}
}
