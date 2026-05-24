package agentclient

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadOrCreateAgentID returns a persistent agent UUID from ~/.onwatch/agent-id,
// creating one if it doesn't exist.
func LoadOrCreateAgentID() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("agentclient.LoadOrCreateAgentID: %w", err)
	}

	dir := filepath.Join(home, ".onwatch")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("agentclient.LoadOrCreateAgentID: mkdir: %w", err)
	}

	path := filepath.Join(dir, "agent-id")
	data, err := os.ReadFile(path)
	if err == nil {
		id := strings.TrimSpace(string(data))
		if len(id) >= 32 {
			return id, nil
		}
	}

	id, err := generateUUID()
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, []byte(id+"\n"), 0600); err != nil {
		return "", fmt.Errorf("agentclient.LoadOrCreateAgentID: write: %w", err)
	}
	return id, nil
}

func generateUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("agentclient.generateUUID: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
