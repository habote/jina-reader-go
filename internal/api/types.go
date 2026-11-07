package api

// PingResponse merepresentasikan payload JSON kembalian endpoint GET /ping
type PingResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Uptime    string `json:"uptime"`
	GitCommit string `json:"git_commit"`
	BuildTime string `json:"build_time"`
}

const (
	DefaultVersion   = "0.1.0"
	DefaultStatus    = "ok"
	DefaultUptime    = "running"
	DefaultGitCommit = "unknown"
	DefaultBuildTime = "unknown"
)

// Package-level variables yang dapat di-override melalui -ldflags saat proses build
var (
	GitCommit = DefaultGitCommit
	BuildTime = DefaultBuildTime
)

// ReadRequest mendefinisikan payload JSON untuk endpoint POST /read
type ReadRequest struct {
	URL string `json:"url"`
}

// ReadResponseJSON mendefinisikan struktur kembalian ketika client meminta Accept: application/json
type ReadResponseJSON struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

// HealthResponse mendefinisikan format JSON endpoint /healthz
type HealthResponse struct {
	Status string `json:"status"`
}
