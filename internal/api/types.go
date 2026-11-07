package api

// PingResponse merepresentasikan payload JSON kembalian endpoint GET /ping
type PingResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

const (
	DefaultVersion = "0.1.0"
	DefaultStatus  = "ok"
	DefaultUptime  = "running"
)
