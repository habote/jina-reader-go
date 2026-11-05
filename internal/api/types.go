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

