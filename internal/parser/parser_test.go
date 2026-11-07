package parser

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestParseHTML_Pruning(t *testing.T) {
	h := `<html><head><title> Judul </title><style>body{color:red}</style><script>var x=1;</script></head>
	<body><nav>Menu Navigasi</nav><h1>Hallo</h1><p>Isi artikel.</p>
	<aside>Sidebar</aside><footer>Footer copy</footer>
	<script>alert(1)</script></body></html>`
	r, err := ParseHTML(strings.NewReader(h))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, want := range []string{"color:red", "var x=1", "Menu Navigasi", "Sidebar", "Footer copy", "alert"} {
		if strings.Contains(r.Markdown, want) {
			t.Errorf("markdown should not contain %q; got:\n%s", want, r.Markdown)
		}
	}
	if !strings.Contains(r.Markdown, "Hallo") || !strings.Contains(r.Markdown, "Isi artikel.") {
		t.Errorf("missing expected content:\n%s", r.Markdown)
	}
}

func TestParseHTML_NestedPrune(t *testing.T) {
	h := `<html><body><script><style>evil</style>more</script><p>ok</p></body></html>`
	r, _ := ParseHTML(strings.NewReader(h))
	if strings.Contains(r.Markdown, "evil") || strings.Contains(r.Markdown, "more") {
		t.Errorf("nested prune leaked: %s", r.Markdown)
	}
	if !strings.Contains(r.Markdown, "ok") {
		t.Errorf("missing ok: %s", r.Markdown)
	}
}

func TestParseHTML_HeadingsAndParagraphs(t *testing.T) {
	h := `<h1>A</h1><h2>B</h2><h6>F</h6><p>Para satu</p><p>Para dua</p>`
	r, _ := ParseHTML(strings.NewReader(h))
	for _, want := range []string{"# A", "## B", "###### F", "Para satu", "Para dua"} {
		if !strings.Contains(r.Markdown, want) {
			t.Errorf("want %q in %q", want, r.Markdown)
		}
	}
}

func TestParseHTML_LinksAndImages(t *testing.T) {
	h := `<p>Lihat <a href="https://example.com">Example</a></p><img src="pic.jpg" alt="Photo">`
	r, _ := ParseHTML(strings.NewReader(h))
	if !strings.Contains(r.Markdown, "[Example](https://example.com)") {
		t.Errorf("link missing: %s", r.Markdown)
	}
	if !strings.Contains(r.Markdown, "![Photo](pic.jpg)") {
		t.Errorf("img missing: %s", r.Markdown)
	}
}

func TestParseHTML_Lists(t *testing.T) {
	h := `<ul><li>satu</li><li>dua</li></ul><ol><li>pertama</li><li>kedua</li></ol>`
	r, _ := ParseHTML(strings.NewReader(h))
	if !strings.Contains(r.Markdown, "- satu") || !strings.Contains(r.Markdown, "- dua") {
		t.Errorf("unordered list missing: %s", r.Markdown)
	}
	if !strings.Contains(r.Markdown, "1. pertama") || !strings.Contains(r.Markdown, "2. kedua") {
		t.Errorf("ordered list missing: %s", r.Markdown)
	}
}

func TestParseHTML_CodeBlocks(t *testing.T) {
	h := "<p>Jalankan <code>go run</code></p><pre>line1\nline2</pre>"
	r, _ := ParseHTML(strings.NewReader(h))
	if !strings.Contains(r.Markdown, "`go run`") {
		t.Errorf("inline code missing: %s", r.Markdown)
	}
	if !strings.Contains(r.Markdown, "```") || !strings.Contains(r.Markdown, "line1") {
		t.Errorf("pre block missing: %s", r.Markdown)
	}
}

func TestParseHTML_InlineFormat(t *testing.T) {
	h := `<p><strong>bold</strong> <em>italic</em> <b>b2</b> <i>i2</i></p>`
	r, _ := ParseHTML(strings.NewReader(h))
	if !strings.Contains(r.Markdown, "**bold**") || !strings.Contains(r.Markdown, "*italic*") {
		t.Errorf("format missing: %s", r.Markdown)
	}
}

func TestParseHTML_Blockquote(t *testing.T) {
	h := `<blockquote>quoted</blockquote>`
	r, _ := ParseHTML(strings.NewReader(h))
	if !strings.Contains(r.Markdown, "> quoted") {
		t.Errorf("blockquote missing: %s", r.Markdown)
	}
}

func TestParseHTML_Malformed(t *testing.T) {
	for _, h := range []string{"<div><p>unclosed", "<p>ok</div>", "<script><style></p>", "plain text", "<b></b><i></i>"} {
		r, err := ParseHTML(strings.NewReader(h))
		if err != nil {
			t.Errorf("unexpected err for %q: %v", h, err)
		}
		if r == nil {
			t.Fatalf("nil result for %q", h)
		}
	}
}

func TestParseHTML_TitleExtraction(t *testing.T) {
	r, _ := ParseHTML(strings.NewReader(`<html><head><title> Judul Saya </title></head><body><p>x</p></body></html>`))
	if strings.TrimSpace(r.Title) != "Judul Saya" {
		t.Errorf("title = %q", r.Title)
	}
	r2, _ := ParseHTML(strings.NewReader(`<html><body><p>x</p></body></html>`))
	if r2.Title != "" {
		t.Errorf("empty title expected, got %q", r2.Title)
	}
}

func TestParseHTML_Table(t *testing.T) {
	h := `<table><tr><th>A</th><th>B</th></tr><tr><td>1</td><td>2</td></tr></table>`
	r, _ := ParseHTML(strings.NewReader(h))
	if !strings.Contains(r.Markdown, "|---|---|") {
		t.Errorf("table separator missing: %s", r.Markdown)
	}
}

func BenchmarkParseHTML(b *testing.B) {
	doc := "<html><head><title>Benchmark</title><style>x{}</style></head><body><h1>T</h1>" +
		strings.Repeat("<p>Paragraf dengan <b>bold</b> dan <a href='https://example.com'>link</a> serta teks panjang.</p>", 50) +
		"<script>alert(1)</script></body></html>"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseHTML(strings.NewReader(doc))
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestParseDocument_DirectDOM(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<html><head><title>DOM</title></head><body><h2>Heading</h2><p>Text <a href="/x">#frag</a> <a href="https://y.test">y</a></p></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	r, err := ParseDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if r.Title != "DOM" {
		t.Errorf("title = %q", r.Title)
	}
	if !strings.Contains(r.Markdown, "## Heading") {
		t.Errorf("heading missing: %s", r.Markdown)
	}
	if !strings.Contains(r.Markdown, "[y](https://y.test)") {
		t.Errorf("external link missing: %s", r.Markdown)
	}
}
