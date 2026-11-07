package parser

import (
	"io"
)

// ParsedResult menyimpan hasil parsing HTML.
type ParsedResult struct {
	Title    string `json:"title"`
	Markdown string `json:"markdown"`
}

// ParseHTML membaca HTML stream dari r dan mengonversinya menjadi
// ParsedResult. Fungsi ini aman terhadap malformed HTML dan tidak
// melempar panic.
func ParseHTML(r io.Reader) (*ParsedResult, error) {
	if r == nil {
		return &ParsedResult{}, nil
	}
	result, err := parse(r)
	if err != nil {
		// Tokenizer hampir tidak pernah error fatal selain EOF/I/O.
		// Pastikan kita tetap mengembalikan hasil yang didapat sejauh ini.
		if result == nil {
			return &ParsedResult{}, nil
		}
		return result, nil
	}
	return result, nil
}
