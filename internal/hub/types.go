package hub

import "time"

// AgentToken represents a hub-issued token for agent authentication.
type AgentToken struct {
	ID         int64
	TokenHash  string
	Prefix     string
	Name       string
	Owner      string
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	Scopes     string
}

// AgentRecord represents a registered agent in the hub database.
type AgentRecord struct {
	ID           int64
	AgentID      string
	TokenID      int64
	Name         string
	Hostname     string
	OS           string
	Arch         string
	AgentVersion string
	FirstSeenAt  time.Time
	LastSeenAt   time.Time
	Providers    string
	Status       string
}

// AgentInfo is the agent self-description sent with each sync request.
type AgentInfo struct {
	Hostname          string   `json:"hostname"`
	OS                string   `json:"os"`
	Arch              string   `json:"arch"`
	Version           string   `json:"version"`
	Mode              string   `json:"mode"`
	DetectedProviders []string `json:"detected_providers"`
}

// CredentialPayload is a single credential sent from agent to hub.
type CredentialPayload struct {
	Provider       string            `json:"provider"`
	AccountName    string            `json:"account_name"`
	CredentialType string            `json:"credential_type"`
	Data           map[string]string `json:"data"`
}

// CostLogModelBreakdown is a per-model cost breakdown within a daily bucket.
type CostLogModelBreakdown struct {
	Model        string  `json:"model"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// CostLogPayload is a daily cost summary from JSONL scanning.
type CostLogPayload struct {
	Provider            string                  `json:"provider"`
	Date                string                  `json:"date"`
	InputTokens         int64                   `json:"input_tokens"`
	OutputTokens        int64                   `json:"output_tokens"`
	CacheReadTokens     int64                   `json:"cache_read_tokens"`
	CacheCreationTokens int64                   `json:"cache_creation_tokens"`
	CostUSD             float64                 `json:"cost_usd"`
	SessionsCount       int                     `json:"sessions_count"`
	ModelsUsed          []string                `json:"models_used"`
	ModelBreakdowns     []CostLogModelBreakdown `json:"model_breakdowns"`
}

// SyncRequest is the payload agents POST to /api/v1/agent/sync.
type SyncRequest struct {
	AgentID     string              `json:"agent_id"`
	AgentInfo   AgentInfo           `json:"agent_info"`
	Credentials []CredentialPayload `json:"credentials,omitempty"`
	CostLogs    []CostLogPayload    `json:"cost_logs,omitempty"`
}

// SyncResponse is the hub's reply to a sync request.
type SyncResponse struct {
	CredentialsAccepted int    `json:"credentials_accepted"`
	CostLogsAccepted    int    `json:"cost_logs_accepted"`
	NextSyncIn          int    `json:"next_sync_in"`
	ServerTime          string `json:"server_time"`
}
