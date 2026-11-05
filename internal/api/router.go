package api

import (
	"net/http"
)

// NewRouter mengembalikan HTTP ServeMux yang sudah terdaftar route-route API.
func NewRouter() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", PingHandler)
	return mux
}
