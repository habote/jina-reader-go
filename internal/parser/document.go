package parser

import (
	"bytes"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// ParseDocument mengonversi DOM hasil Colly/goquery menjadi Markdown.
//
// Aturan konversi identik dengan implementasi streaming lama:
// prune script/style/noscript/svg/nav/footer/aside/iframe/head/form/
// button/select/textarea/input, heading, paragraf, link, gambar,
// list, code block, blockquote, tabel, bold/italic, hr, br, dan
// ekstraksi <title>. Aman terhadap HTML malformed karena
// goquery/html sudah menormalisasinya saat parse DOM.
func ParseDocument(doc *goquery.Document) (*ParsedResult, error) {
	if doc == nil {
		return &ParsedResult{}, nil
	}

	d := &docConverter{buf: getBuffer()}
	defer putBuffer(d.buf)

	title := cleanText(doc.Find("title").First().Text())

	clone := doc.Clone()
	clone.Find(pruneSelector).Remove()

	// Walk hanya subtree <body> (head sudah di-prune kecuali title yang diambil di atas).
	// Jika tidak ada <body> (HTML fragmen), fallback ke <html>.
	if clone.Find("body").Length() > 0 {
		clone.Find("body").Each(func(_ int, s *goquery.Selection) {
			d.walk(s)
		})
	} else if clone.Find("html").Length() > 0 {
		d.walk(clone.Find("html"))
	}

	markdown := strings.TrimSpace(normalizeMarkdown(d.buf.String()))
	if markdown == "" && title != "" {
		markdown = title
	}
	return &ParsedResult{Title: title, Markdown: markdown}, nil
}

type listContext struct {
	ordered bool
	index   int
}

const pruneSelector = "script, style, noscript, svg, nav, footer, aside, iframe, head, form, button, select, textarea, input"

type docConverter struct {
	buf        *bytes.Buffer
	inPre      bool
	inCode     bool
	inLink     bool
	linkHref   string
	linkText   *bytes.Buffer
	listStack  []listContext
	blockDepth int
	tableRow   int
	tableHead  bool
}

func (d *docConverter) ensureDoubleNL() {
	if d.buf.Len() == 0 {
		return
	}
	b := d.buf.Bytes()
	if len(b) >= 2 && b[len(b)-1] == '\n' && b[len(b)-2] == '\n' {
		return
	}
	if b[len(b)-1] == '\n' {
		d.buf.WriteByte('\n')
		return
	}
	d.buf.WriteString("\n\n")
}

func (d *docConverter) ensureNL() {
	if d.buf.Len() == 0 {
		return
	}
	if d.buf.Bytes()[d.buf.Len()-1] != '\n' {
		d.buf.WriteByte('\n')
	}
}

// walk memproses satu node dan anak-anaknya secara rekursif.
func (d *docConverter) walk(s *goquery.Selection) {
	for _, n := range s.Nodes {
		d.walkNode(n)
	}
}

func (d *docConverter) walkNode(n *html.Node) {
	if n.Type == html.TextNode {
		d.writeText(n.Data)
		return
	}
	if n.Type != html.ElementNode {
		return
	}

	switch n.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		d.ensureDoubleNL()
		level := int(n.Data[1] - '0')
		d.buf.WriteString(strings.Repeat("#", level) + " ")
		d.walkChildren(n)
		d.ensureDoubleNL()
	case "p", "div", "article", "section", "header", "main":
		d.ensureDoubleNL()
		d.walkChildren(n)
		d.ensureDoubleNL()
	case "br":
		d.buf.WriteByte('\n')
	case "hr":
		d.ensureDoubleNL()
		d.buf.WriteString("---\n")
		d.ensureDoubleNL()
	case "a":
		href, _ := getAttr(n, "href")
		d.inLink = true
		d.linkHref = strings.TrimSpace(href)
		d.linkText = getBuffer()
		d.walkChildren(n)
		text := strings.TrimSpace(cleanInline(d.linkText.String()))
		putBuffer(d.linkText)
		d.linkText = nil
		href = d.linkHref
		d.inLink = false
		d.linkHref = ""
		if href != "" && !strings.HasPrefix(href, "#") && text != "" {
			d.buf.WriteString("[" + text + "](" + href + ")")
		} else {
			d.buf.WriteString(text)
		}
	case "img":
		src, _ := getAttr(n, "src")
		alt, _ := getAttr(n, "alt")
		if src != "" {
			d.ensureDoubleNL()
			d.buf.WriteString("![" + cleanInline(alt) + "](" + src + ")\n\n")
		}
	case "b", "strong":
		d.buf.WriteString("**")
		d.walkChildren(n)
		d.buf.WriteString("**")
	case "i", "em":
		d.buf.WriteString("*")
		d.walkChildren(n)
		d.buf.WriteString("*")
	case "pre":
		d.inPre = true
		d.ensureDoubleNL()
		d.buf.WriteString("```\n")
		d.walkChildren(n)
		d.buf.WriteString("\n```\n")
		d.ensureDoubleNL()
		d.inPre = false
	case "code":
		d.inCode = true
		if !d.inPre {
			d.buf.WriteString("`")
		}
		d.walkChildren(n)
		d.inCode = false
		if !d.inPre {
			d.buf.WriteString("`")
		}
	case "blockquote":
		d.blockDepth++
		d.ensureDoubleNL()
		d.buf.WriteString("> ")
		d.walkChildren(n)
		d.ensureDoubleNL()
		d.blockDepth--
	case "ul":
		d.listStack = append(d.listStack, listContext{ordered: false})
		d.walkChildren(n)
		d.listStack = d.listStack[:len(d.listStack)-1]
		d.ensureDoubleNL()
	case "ol":
		d.listStack = append(d.listStack, listContext{ordered: true, index: 0})
		d.walkChildren(n)
		d.listStack = d.listStack[:len(d.listStack)-1]
		d.ensureDoubleNL()
	case "li":
		d.ensureNL()
		if len(d.listStack) > 1 {
			d.buf.WriteString(strings.Repeat("  ", len(d.listStack)-1))
		}
		if len(d.listStack) > 0 && d.listStack[len(d.listStack)-1].ordered {
			d.listStack[len(d.listStack)-1].index++
			d.buf.WriteString(itoa(d.listStack[len(d.listStack)-1].index) + ". ")
		} else {
			d.buf.WriteString("- ")
		}
		d.walkChildren(n)
		d.ensureNL()
	case "table":
		d.ensureDoubleNL()
		d.tableRow = 0
		d.walkChildren(n)
		d.ensureDoubleNL()
	case "tr":
		d.tableHead = false
		d.buf.WriteString("|")
		d.walkChildren(n)
		if d.tableHead {
			d.buf.WriteString("\n|---|---|")
		}
		d.buf.WriteString("\n")
		d.tableRow++
	case "th":
		d.tableHead = true
		d.buf.WriteString(" ")
		d.walkChildren(n)
		d.buf.WriteString(" |")
	case "td":
		d.buf.WriteString(" ")
		d.walkChildren(n)
		d.buf.WriteString(" |")
	default:
		d.walkChildren(n)
	}
}

func (d *docConverter) walkChildren(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		d.walkNode(c)
	}
}

func (d *docConverter) writeText(s string) {
	if d.inPre || d.inCode {
		if d.inLink && d.linkText != nil {
			d.linkText.WriteString(s)
			return
		}
		d.buf.WriteString(s)
		return
	}
	cleaned := cleanText(s)
	if cleaned == "" {
		return
	}
	if d.inLink && d.linkText != nil {
		d.linkText.WriteString(cleaned)
		return
	}
	d.buf.WriteString(cleaned)
}

func getAttr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val, true
		}
	}
	return "", false
}

// cleanText dan cleanInline dipertahankan dari tokenizer lama agar
// normalisasi whitespace & entity tetap identik.
func cleanText(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

func cleanInline(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = html.UnescapeString(s)
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

func normalizeMarkdown(s string) string {
	s = strings.ReplaceAll(s, "****", "")
	s = strings.ReplaceAll(s, "__", "")
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}

func itoa(n int) string {
	return intToStr(n)
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
