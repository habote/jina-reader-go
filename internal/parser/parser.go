package parser

import (
	"io"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ParsedResult menyimpan hasil parsing HTML.
type ParsedResult struct {
	Title    string `json:"title"`
	Markdown string `json:"markdown"`
}

// ParseHTML membaca HTML stream dari r dan mengonversinya menjadi
// ParsedResult. Fungsi ini aman terhadap malformed HTML dan tidak
// melempar panic.
//
// Implementasi lama memakai streaming tokenizer; sejak refactor Colly
// (issue #16) fungsi ini menjadi adaptor di atas DOM goquery agar
// kontrak internal/api/handler.go tidak berubah.
func ParseHTML(r io.Reader) (*ParsedResult, error) {
	if r == nil {
		return &ParsedResult{}, nil
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return &ParsedResult{}, nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(data)))
	if err != nil {
		return &ParsedResult{}, nil
	}
	return ParseDocument(doc)
}
