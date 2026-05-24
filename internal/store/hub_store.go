package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/hub"
)

func (s *Store) CreateAgentToken(tokenHash, prefix, name, owner, scopes string, expiresAt *time.Time) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	var expiresStr *string
	if expiresAt != nil {
		s := expiresAt.UTC().Format(time.RFC3339)
		expiresStr = &s
	}
	result, err := s.db.Exec(`
		INSERT INTO agent_tokens (token_hash, token_prefix, name, owner, created_at, expires_at, scopes)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, tokenHash, prefix, name, owner, now, expiresStr, scopes)
	if err != nil {
		return 0, fmt.Errorf("store.CreateAgentToken: %w", err)
	}
	return result.LastInsertId()
}

func (s *Store) GetAgentTokenByHash(tokenHash string) (*hub.AgentToken, error) {
	row := s.db.QueryRow(`
		SELECT id, token_hash, token_prefix, name, owner, created_at, expires_at, last_used_at, revoked_at, scopes
		FROM agent_tokens WHERE token_hash = ?
	`, tokenHash)
	return scanAgentToken(row)
}

func (s *Store) UpdateAgentTokenLastUsed(id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE agent_tokens SET last_used_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return fmt.Errorf("store.UpdateAgentTokenLastUsed: %w", err)
	}
	return nil
}

func (s *Store) ListAgentTokens() ([]hub.AgentToken, error) {
	rows, err := s.db.Query(`
		SELECT id, token_hash, token_prefix, name, owner, created_at, expires_at, last_used_at, revoked_at, scopes
		FROM agent_tokens ORDER BY created_at DESC LIMIT 200
	`)
	if err != nil {
		return nil, fmt.Errorf("store.ListAgentTokens: %w", err)
	}
	defer rows.Close()

	var tokens []hub.AgentToken
	for rows.Next() {
		t, err := scanAgentTokenRow(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, *t)
	}
	return tokens, rows.Err()
}

func (s *Store) RevokeAgentToken(id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(`UPDATE agent_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, now, id)
	if err != nil {
		return fmt.Errorf("store.RevokeAgentToken: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("store.RevokeAgentToken: token not found or already revoked")
	}
	return nil
}

func (s *Store) RevokeAgentTokenByName(name string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(`UPDATE agent_tokens SET revoked_at = ? WHERE name = ? AND revoked_at IS NULL`, now, name)
	if err != nil {
		return fmt.Errorf("store.RevokeAgentTokenByName: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("store.RevokeAgentTokenByName: token %q not found or already revoked", name)
	}
	return nil
}

func (s *Store) RegisterOrUpdateAgent(agentID string, tokenID int64, info hub.AgentInfo) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	providersJSON, _ := json.Marshal(info.DetectedProviders)

	var id int64
	err := s.db.QueryRow(`SELECT id FROM agents WHERE agent_id = ?`, agentID).Scan(&id)
	if err == sql.ErrNoRows {
		name := info.Hostname
		if name == "" {
			name = agentID[:8]
		}
		result, insertErr := s.db.Exec(`
			INSERT INTO agents (agent_id, token_id, name, hostname, os, arch, agent_version, first_seen_at, last_seen_at, providers, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active')
		`, agentID, tokenID, name, info.Hostname, info.OS, info.Arch, info.Version, now, now, string(providersJSON))
		if insertErr != nil {
			return 0, fmt.Errorf("store.RegisterOrUpdateAgent insert: %w", insertErr)
		}
		return result.LastInsertId()
	}
	if err != nil {
		return 0, fmt.Errorf("store.RegisterOrUpdateAgent lookup: %w", err)
	}

	_, err = s.db.Exec(`
		UPDATE agents SET hostname = ?, os = ?, arch = ?, agent_version = ?, last_seen_at = ?, providers = ?, status = 'active'
		WHERE id = ? AND token_id = ?
	`, info.Hostname, info.OS, info.Arch, info.Version, now, string(providersJSON), id, tokenID)
	if err != nil {
		return 0, fmt.Errorf("store.RegisterOrUpdateAgent update: %w", err)
	}
	return id, nil
}

func (s *Store) UpdateAgentHeartbeat(agentID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE agents SET last_seen_at = ?, status = 'active' WHERE agent_id = ?`, now, agentID)
	if err != nil {
		return fmt.Errorf("store.UpdateAgentHeartbeat: %w", err)
	}
	return nil
}

func (s *Store) GetAgent(agentID string) (*hub.AgentRecord, error) {
	row := s.db.QueryRow(`
		SELECT id, agent_id, token_id, name, hostname, os, arch, agent_version, first_seen_at, last_seen_at, providers, status
		FROM agents WHERE agent_id = ?
	`, agentID)
	return scanAgentRecord(row)
}

func (s *Store) GetAgentByID(id int64) (*hub.AgentRecord, error) {
	row := s.db.QueryRow(`
		SELECT id, agent_id, token_id, name, hostname, os, arch, agent_version, first_seen_at, last_seen_at, providers, status
		FROM agents WHERE id = ?
	`, id)
	return scanAgentRecord(row)
}

func (s *Store) ListAgents() ([]hub.AgentRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, agent_id, token_id, name, hostname, os, arch, agent_version, first_seen_at, last_seen_at, providers, status
		FROM agents ORDER BY last_seen_at DESC LIMIT 200
	`)
	if err != nil {
		return nil, fmt.Errorf("store.ListAgents: %w", err)
	}
	defer rows.Close()

	var agents []hub.AgentRecord
	for rows.Next() {
		a, err := scanAgentRecordRow(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, *a)
	}
	return agents, rows.Err()
}

func (s *Store) DeleteAgent(id int64) error {
	_, err := s.db.Exec(`UPDATE agents SET status = 'deregistered' WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store.DeleteAgent: %w", err)
	}
	return nil
}

func (s *Store) UpsertAgentCredential(agentDBID int64, provider, accountName, credentialType, encryptedData string, expiresAt *time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var expiresStr *string
	if expiresAt != nil {
		s := expiresAt.UTC().Format(time.RFC3339)
		expiresStr = &s
	}
	_, err := s.db.Exec(`
		INSERT INTO agent_credentials (agent_id, provider, account_name, credential_type, encrypted_data, updated_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id, provider, account_name) DO UPDATE SET
			credential_type = excluded.credential_type,
			encrypted_data = excluded.encrypted_data,
			updated_at = excluded.updated_at,
			expires_at = excluded.expires_at
	`, agentDBID, provider, accountName, credentialType, encryptedData, now, expiresStr)
	if err != nil {
		return fmt.Errorf("store.UpsertAgentCredential: %w", err)
	}
	return nil
}

func (s *Store) UpsertAgentCostLog(agentDBID int64, log hub.CostLogPayload) error {
	now := time.Now().UTC().Format(time.RFC3339)
	modelsJSON, _ := json.Marshal(log.ModelsUsed)
	breakdownsJSON, _ := json.Marshal(log.ModelBreakdowns)
	_, err := s.db.Exec(`
		INSERT INTO agent_cost_logs (agent_id, provider, date, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, cost_usd, sessions_count, models_used, model_breakdowns, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id, provider, date) DO UPDATE SET
			input_tokens = excluded.input_tokens,
			output_tokens = excluded.output_tokens,
			cache_read_tokens = excluded.cache_read_tokens,
			cache_creation_tokens = excluded.cache_creation_tokens,
			cost_usd = excluded.cost_usd,
			sessions_count = excluded.sessions_count,
			models_used = excluded.models_used,
			model_breakdowns = excluded.model_breakdowns,
			updated_at = excluded.updated_at
	`, agentDBID, log.Provider, log.Date, log.InputTokens, log.OutputTokens, log.CacheReadTokens, log.CacheCreationTokens, log.CostUSD, log.SessionsCount, string(modelsJSON), string(breakdownsJSON), now)
	if err != nil {
		return fmt.Errorf("store.UpsertAgentCostLog: %w", err)
	}
	return nil
}

// scanners

type scannable interface {
	Scan(dest ...any) error
}

func scanAgentToken(row scannable) (*hub.AgentToken, error) {
	var t hub.AgentToken
	var createdStr string
	var expiresStr, lastUsedStr, revokedStr sql.NullString
	err := row.Scan(&t.ID, &t.TokenHash, &t.Prefix, &t.Name, &t.Owner, &createdStr, &expiresStr, &lastUsedStr, &revokedStr, &t.Scopes)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("store.scanAgentToken: %w", err)
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	if expiresStr.Valid {
		parsed, _ := time.Parse(time.RFC3339, expiresStr.String)
		t.ExpiresAt = &parsed
	}
	if lastUsedStr.Valid {
		parsed, _ := time.Parse(time.RFC3339, lastUsedStr.String)
		t.LastUsedAt = &parsed
	}
	if revokedStr.Valid {
		parsed, _ := time.Parse(time.RFC3339, revokedStr.String)
		t.RevokedAt = &parsed
	}
	return &t, nil
}

func scanAgentTokenRow(rows *sql.Rows) (*hub.AgentToken, error) {
	return scanAgentToken(rows)
}

func scanAgentRecord(row scannable) (*hub.AgentRecord, error) {
	var a hub.AgentRecord
	var firstStr, lastStr string
	err := row.Scan(&a.ID, &a.AgentID, &a.TokenID, &a.Name, &a.Hostname, &a.OS, &a.Arch, &a.AgentVersion, &firstStr, &lastStr, &a.Providers, &a.Status)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("store.scanAgentRecord: %w", err)
	}
	a.FirstSeenAt, _ = time.Parse(time.RFC3339, firstStr)
	a.LastSeenAt, _ = time.Parse(time.RFC3339, lastStr)
	return &a, nil
}

func scanAgentRecordRow(rows *sql.Rows) (*hub.AgentRecord, error) {
	return scanAgentRecord(rows)
}
