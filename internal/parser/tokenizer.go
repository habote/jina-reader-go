package parser

import (
	"bytes"
	"io"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type listContext struct {
	ordered bool
	index   int
}

var pruneTags = map[atom.Atom]bool{
	atom.Script:   true,
	atom.Style:    true,
	atom.Noscript: true,
	atom.Svg:      true,
	atom.Nav:      true,
	atom.Footer:   true,
	atom.Aside:    true,
	atom.Iframe:   true,
	atom.Head:     true,
	atom.Form:     true,
	atom.Button:   true,
	atom.Select:   true,
	atom.Textarea: true,
	atom.Input:    true,
}

type parseState struct {
	buf        *bytes.Buffer
	titleBuf   *bytes.Buffer
	linkText   *bytes.Buffer
	skipDepth  int
	inTitle    bool
	inPre      bool
	inCode     bool
	inBlock    int
	inLink     bool
	linkHref   string
	listStack  []listContext
	tblRows    int
	tblInRow   bool
	tblHeaders bool
}

func parse(r io.Reader) (*ParsedResult, error) {
	buf := getBuffer()
	defer putBuffer(buf)
	titleBuf := getBuffer()
	defer putBuffer(titleBuf)
	linkText := getBuffer()
	defer putBuffer(linkText)

	s := &parseState{buf: buf, titleBuf: titleBuf, linkText: linkText}
	z := html.NewTokenizer(r)

	ensureNewline := func(n int) {
		for i := 0; i < n; i++ {
			if buf.Len() > 0 && !bytes.HasSuffix(buf.Bytes(), []byte("\n")) {
				buf.WriteByte('\n')
			} else if buf.Len() == 0 {
				break
			}
		}
	}

	hasDoubleNL := func() bool {
		b := buf.Bytes()
		if len(b) < 2 {
			return false
		}
		return b[len(b)-1] == '\n' && b[len(b)-2] == '\n'
	}

	ensureDoubleNL := func() {
		if buf.Len() == 0 {
			return
		}
		if !bytes.HasSuffix(buf.Bytes(), []byte("\n")) {
			buf.WriteByte('\n')
		}
		if !hasDoubleNL() {
			buf.WriteByte('\n')
		}
	}

	_ = ensureNewline

	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}

		switch tt {
		case html.StartTagToken:
			tok := z.Token()
			tokAttrs := map[string]string{}
			for _, at := range tok.Attr {
				tokAttrs[strings.ToLower(at.Key)] = at.Val
			}
			a := tok.DataAtom
			hasAttr := len(tok.Attr) > 0
			_ = hasAttr
			if a == atom.Title && s.skipDepth > 0 {
				s.inTitle = true
				continue
			}
			if pruneTags[a] {
				s.skipDepth++
				continue
			}
			if s.skipDepth > 0 {
				continue
			}
			switch a {
			case atom.Title:
				s.inTitle = true
			case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
				ensureDoubleNL()
				level := int(a - atom.H1 + 1)
				buf.WriteString(strings.Repeat("#", level) + " ")
			case atom.P, atom.Article, atom.Section, atom.Header, atom.Div, atom.Main:
				ensureDoubleNL()
			case atom.Br:
				buf.WriteByte('\n')
			case atom.Hr:
				ensureDoubleNL()
				buf.WriteString("---\n")
				ensureDoubleNL()
			case atom.A:
				href := ""
				if hasAttr {
					href = tokAttrs["href"]
				}
				s.inLink = true
				s.linkHref = strings.TrimSpace(href)
				s.linkText.Reset()
			case atom.Img:
				src, alt := "", ""
				if hasAttr {
					src = tokAttrs["src"]
					alt = tokAttrs["alt"]
				}
				if src != "" {
					ensureDoubleNLIfNeeded(buf)
					buf.WriteString("![")
					buf.WriteString(cleanInline(alt))
					buf.WriteString("](")
					buf.WriteString(src)
					buf.WriteString(")\n\n")
				}
			case atom.B, atom.Strong:
				if !emptyAhead(z) {
					buf.WriteString("**")
				}
			case atom.I, atom.Em:
				if !emptyAhead(z) {
					buf.WriteString("*")
				}
			case atom.Pre:
				s.inPre = true
				ensureDoubleNL()
				buf.WriteString("```\n")
			case atom.Code:
				s.inCode = true
				if !s.inPre {
					buf.WriteString("`")
				}
			case atom.Blockquote:
				s.inBlock++
				ensureDoubleNL()
				buf.WriteString("> ")
			case atom.Ul:
				s.listStack = append(s.listStack, listContext{ordered: false})
			case atom.Ol:
				s.listStack = append(s.listStack, listContext{ordered: true, index: 0})
			case atom.Li:
				ensureDoubleNLIfNeeded(buf)
				indent := ""
				if n := len(s.listStack); n > 1 {
					indent = strings.Repeat("  ", n-1)
				}
				buf.WriteString(indent)
				if len(s.listStack) > 0 && s.listStack[len(s.listStack)-1].ordered {
					s.listStack[len(s.listStack)-1].index++
					buf.WriteString(itoa(s.listStack[len(s.listStack)-1].index) + ". ")
				} else {
					buf.WriteString("- ")
				}
			case atom.Table:
				ensureDoubleNL()
				s.tblRows = 0
			case atom.Tr:
				s.tblInRow = true
				s.tblHeaders = false
				buf.WriteString("|")
			case atom.Th:
				s.tblHeaders = true
				buf.WriteString(" ")
			case atom.Td:
				buf.WriteString(" ")
			}
		case html.EndTagToken:
			tok := z.Token()
			a := tok.DataAtom
			if a == atom.Title && s.inTitle {
				s.inTitle = false
				continue
			}
			if pruneTags[a] {
				if s.skipDepth > 0 {
					s.skipDepth--
				}
				continue
			}
			if s.skipDepth > 0 {
				continue
			}
			switch a {
			case atom.Title:
				s.inTitle = false
			case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
				ensureDoubleNL()
			case atom.P, atom.Article, atom.Section, atom.Header, atom.Div, atom.Main:
				ensureDoubleNL()
			case atom.A:
				if s.inLink {
					text := strings.TrimSpace(cleanInline(s.linkText.String()))
					href := s.linkHref
					if href != "" && !strings.HasPrefix(href, "#") {
						if text != "" {
							buf.WriteString("[")
							buf.WriteString(text)
							buf.WriteString("](")
							buf.WriteString(href)
							buf.WriteString(")")
						}
					} else if text != "" {
						buf.WriteString(text)
					}
					s.inLink = false
					s.linkHref = ""
					s.linkText.Reset()
				}
			case atom.B, atom.Strong:
				buf.WriteString("**")
			case atom.I, atom.Em:
				buf.WriteString("*")
			case atom.Pre:
				s.inPre = false
				buf.WriteString("\n```\n")
				ensureDoubleNL()
			case atom.Code:
				s.inCode = false
				if !s.inPre {
					buf.WriteString("`")
				}
			case atom.Blockquote:
				if s.inBlock > 0 {
					s.inBlock--
				}
				ensureDoubleNL()
			case atom.Ul, atom.Ol:
				if len(s.listStack) > 0 {
					s.listStack = s.listStack[:len(s.listStack)-1]
				}
				ensureDoubleNL()
			case atom.Tr:
				if s.tblHeaders {
					buf.WriteString("\n|---|---|")
				}
				buf.WriteString("\n")
				s.tblRows++
				s.tblInRow = false
			case atom.Th, atom.Td:
				buf.WriteString(" |")
			case atom.Table:
				ensureDoubleNL()
			}
		case html.SelfClosingTagToken:
			tok := z.Token()
			tokAttrs := map[string]string{}
			for _, at := range tok.Attr {
				tokAttrs[strings.ToLower(at.Key)] = at.Val
			}
			a := tok.DataAtom
			hasAttr := len(tok.Attr) > 0
			_ = hasAttr
			if pruneTags[a] || s.skipDepth > 0 {
				continue
			}
			switch a {
			case atom.Br:
				buf.WriteByte('\n')
			case atom.Hr:
				ensureDoubleNL()
				buf.WriteString("---\n")
				ensureDoubleNL()
			case atom.Img:
				src, alt := "", ""
				if hasAttr {
					src = tokAttrs["src"]
					alt = tokAttrs["alt"]
				}
				if src != "" {
					buf.WriteString("![")
					buf.WriteString(cleanInline(alt))
					buf.WriteString("](")
					buf.WriteString(src)
					buf.WriteString(")\n\n")
				}
			case atom.Input, atom.Meta, atom.Link:
				// abaikan
			}
		case html.TextToken:
			if s.inTitle {
				// captured di bawah; jangan skip meski skipDepth>0 (title dalam head)
			} else if s.skipDepth > 0 {
				continue
			}
			text := string(z.Text())
			if s.inTitle {
				s.titleBuf.WriteString(text)
				continue
			}
			if s.inPre || s.inCode {
				if s.inLink {
					s.linkText.WriteString(text)
				} else {
					buf.WriteString(text)
				}
				continue
			}
			cleaned := cleanText(text)
			if cleaned == "" {
				continue
			}
			if s.inLink {
				s.linkText.WriteString(cleaned)
				continue
			}
			buf.WriteString(cleaned)
		case html.CommentToken:
			// abaikan komentar
		}
	}

	title := strings.TrimSpace(cleanInline(titleBuf.String()))
	markdown := strings.TrimSpace(normalizeMarkdown(buf.String()))
	if markdown == "" && title != "" {
		markdown = title
	}
	return &ParsedResult{Title: title, Markdown: markdown}, nil
}

func emptyAhead(z *html.Tokenizer) bool {
	// Best-effort: peek via buffering is complex; treat as non-empty.
	// Formatter kosong (mis. <b></b>) menghasilkan "****" yang kemudian
	// dinormalisasi di normalizeMarkdown.
	return false
}

func cleanText(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = html.UnescapeString(s)
	// collapse whitespace dalam text token (tidak untuk pre/code sudah ditangani terpisah)
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return ""
	}
	return s
}

func cleanInline(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

func normalizeMarkdown(s string) string {
	// hapus formatter kosong **** / **
	// (gambaran sederhana: cukup untuk memenuhi edge case)
	s = strings.ReplaceAll(s, "****", "")
	s = strings.ReplaceAll(s, "__", "")
	// rapikan newline berlebih
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}

func ensureDoubleNLIfNeeded(buf *bytes.Buffer) {
	if buf.Len() == 0 {
		return
	}
	b := buf.Bytes()
	if len(b) >= 2 && b[len(b)-2] == '\n' && b[len(b)-1] == '\n' {
		return
	}
	if len(b) >= 1 && b[len(b)-1] == '\n' {
		buf.WriteByte('\n')
		return
	}
	buf.WriteString("\n\n")
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
