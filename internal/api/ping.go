package api

import (
	"encoding/json"
	"net/http"
)

// PingHandler menangani request HTTP GET /ping dan mengembalikan informasi status, version, dan uptime.
func PingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := PingResponse{
		Status:    DefaultStatus,
		Version:   DefaultVersion,
		Uptime:    DefaultUptime,
		GitCommit: GitCommit,
		BuildTime: BuildTime,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
