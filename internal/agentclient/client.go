package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/hub"
)

// Client syncs data from agent to hub.
type Client struct {
	hubURL string
	token  string
	http   *http.Client
}

// NewClient creates a hub sync client.
func NewClient(hubURL, token string) *Client {
	return &Client{
		hubURL: hubURL,
		token:  token,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Sync posts credentials and cost logs to the hub.
func (c *Client) Sync(ctx context.Context, req *hub.SyncRequest) (*hub.SyncResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("agentclient.Sync: marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.hubURL+"/api/v1/agent/sync", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("agentclient.Sync: new request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("agentclient.Sync: do: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("agentclient.Sync: unauthorized - check agent token")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agentclient.Sync: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var syncResp hub.SyncResponse
	if err := json.Unmarshal(respBody, &syncResp); err != nil {
		return nil, fmt.Errorf("agentclient.Sync: decode response: %w", err)
	}
	return &syncResp, nil
}

// Heartbeat sends a heartbeat to the hub.
func (c *Client) Heartbeat(ctx context.Context, agentID string) error {
	body, _ := json.Marshal(map[string]string{"agent_id": agentID})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.hubURL+"/api/v1/agent/heartbeat", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("agentclient.Heartbeat: new request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("agentclient.Heartbeat: do: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agentclient.Heartbeat: status %d", resp.StatusCode)
	}
	return nil
}

// SyncWithRetry attempts to sync with exponential backoff.
func (c *Client) SyncWithRetry(ctx context.Context, req *hub.SyncRequest, maxRetries int) (*hub.SyncResponse, error) {
	backoff := 30 * time.Second
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := c.Sync(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > 10*time.Minute {
				backoff = 10 * time.Minute
			}
		}
	}
	return nil, fmt.Errorf("agentclient.SyncWithRetry: all %d attempts failed, last error: %w", maxRetries+1, lastErr)
}
