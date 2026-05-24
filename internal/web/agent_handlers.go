package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/hub"
)

// AgentSync handles POST /api/v1/agent/sync from onwatch-agent instances.
func (h *Handler) AgentSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	tokenID, ok := hub.TokenIDFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req hub.SyncRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.AgentID == "" {
		http.Error(w, `{"error":"agent_id required"}`, http.StatusBadRequest)
		return
	}

	agentDBID, err := h.store.RegisterOrUpdateAgent(req.AgentID, tokenID, req.AgentInfo)
	if err != nil {
		h.logger.Error("agent sync: register failed", "error", err, "agent_id", req.AgentID)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	var credsAccepted, costLogsAccepted int

	for _, cred := range req.Credentials {
		if cred.Provider == "" || cred.CredentialType == "" {
			continue
		}
		dataJSON, _ := json.Marshal(cred.Data)
		// TODO: encrypt with HKDF before storing (Phase 2 hardening)
		err := h.store.UpsertAgentCredential(agentDBID, cred.Provider, cred.AccountName, cred.CredentialType, string(dataJSON), nil)
		if err != nil {
			h.logger.Error("agent sync: credential store failed", "error", err, "provider", cred.Provider)
			continue
		}
		credsAccepted++
	}

	for _, cl := range req.CostLogs {
		if cl.Provider == "" || cl.Date == "" {
			continue
		}
		err := h.store.UpsertAgentCostLog(agentDBID, cl)
		if err != nil {
			h.logger.Error("agent sync: cost log store failed", "error", err, "provider", cl.Provider, "date", cl.Date)
			continue
		}
		costLogsAccepted++
	}

	resp := hub.SyncResponse{
		CredentialsAccepted: credsAccepted,
		CostLogsAccepted:    costLogsAccepted,
		NextSyncIn:          120,
		ServerTime:          time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// AgentHeartbeat handles POST /api/v1/agent/heartbeat from agents.
func (h *Handler) AgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	tokenID, ok := hub.TokenIDFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.AgentID == "" {
		http.Error(w, `{"error":"agent_id required"}`, http.StatusBadRequest)
		return
	}

	// Verify the agent belongs to this token
	agent, err := h.store.GetAgent(req.AgentID)
	if err != nil || agent == nil {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}
	if agent.TokenID != tokenID {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusForbidden)
		return
	}

	if err := h.store.UpdateAgentHeartbeat(req.AgentID); err != nil {
		h.logger.Error("agent heartbeat failed", "error", err, "agent_id", req.AgentID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":      "ok",
		"server_time": time.Now().UTC().Format(time.RFC3339),
	})
}

// AdminAgents handles GET/DELETE /api/v1/admin/agents for dashboard management.
func (h *Handler) AdminAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		agents, err := h.store.ListAgents()
		if err != nil {
			h.logger.Error("admin agents list failed", "error", err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agents)

	case http.MethodDelete:
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			http.Error(w, `{"error":"id parameter required"}`, http.StatusBadRequest)
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
			return
		}
		if err := h.store.DeleteAgent(id); err != nil {
			h.logger.Error("admin agent delete failed", "error", err, "id", id)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deregistered"})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// AdminTokens handles GET/POST/DELETE /api/v1/admin/tokens for dashboard token management.
func (h *Handler) AdminTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tokens, err := h.store.ListAgentTokens()
		if err != nil {
			h.logger.Error("admin tokens list failed", "error", err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokens)

	case http.MethodPost:
		var req struct {
			Name      string  `json:"name"`
			Owner     string  `json:"owner"`
			ExpiresIn *string `json:"expires_in"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
			return
		}

		raw, err := hub.GenerateRawToken()
		if err != nil {
			h.logger.Error("admin token create failed", "error", err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}

		tokenHash := hub.HashToken(raw)
		prefix := hub.TokenDisplayPrefix(raw)

		_, err = h.store.CreateAgentToken(tokenHash, prefix, req.Name, req.Owner, "sync", nil)
		if err != nil {
			h.logger.Error("admin token store failed", "error", err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"token": raw,
			"name":  req.Name,
		})

	case http.MethodDelete:
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			http.Error(w, `{"error":"id parameter required"}`, http.StatusBadRequest)
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
			return
		}
		if err := h.store.RevokeAgentToken(id); err != nil {
			h.logger.Error("admin token revoke failed", "error", err, "id", id)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "revoked"})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}
