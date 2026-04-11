package api

const (
	ProtocolVersion = "v0"
	DaemonVersion   = "dev"
)

type PingRequest struct {
	ProtocolVersion string `json:"protocol_version"`
}

type PingResponse struct {
	ProtocolVersion string `json:"protocol_version"`
	DaemonVersion   string `json:"daemon_version"`
}
