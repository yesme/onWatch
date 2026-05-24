package agentclient

import (
	"testing"
)

func TestLoadConfig_CLIFlags(t *testing.T) {
	cfg, err := LoadConfig([]string{
		"--hub", "https://hub.example.com",
		"--token", "owt_abc123",
		"--name", "test-machine",
		"--mode", "token-sync",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HubURL != "https://hub.example.com" {
		t.Errorf("expected hub URL 'https://hub.example.com', got %q", cfg.HubURL)
	}
	if cfg.AgentToken != "owt_abc123" {
		t.Errorf("expected token 'owt_abc123', got %q", cfg.AgentToken)
	}
	if cfg.AgentName != "test-machine" {
		t.Errorf("expected name 'test-machine', got %q", cfg.AgentName)
	}
	if cfg.Mode != ModeTokenSync {
		t.Errorf("expected mode 'token-sync', got %q", cfg.Mode)
	}
}

func TestLoadConfig_MissingHub(t *testing.T) {
	_, err := LoadConfig([]string{"--token", "owt_abc123"})
	if err == nil {
		t.Error("expected error for missing hub URL")
	}
}

func TestLoadConfig_MissingToken(t *testing.T) {
	_, err := LoadConfig([]string{"--hub", "https://hub.example.com"})
	if err == nil {
		t.Error("expected error for missing token")
	}
}

func TestLoadConfig_InvalidMode(t *testing.T) {
	_, err := LoadConfig([]string{
		"--hub", "https://hub.example.com",
		"--token", "owt_abc",
		"--mode", "invalid",
	})
	if err == nil {
		t.Error("expected error for invalid mode")
	}
}

func TestLoadConfig_NonHTTPS_Rejected(t *testing.T) {
	_, err := LoadConfig([]string{
		"--hub", "http://remote.example.com",
		"--token", "owt_abc",
	})
	if err == nil {
		t.Error("expected error for non-HTTPS hub")
	}
}

func TestLoadConfig_Localhost_Allowed(t *testing.T) {
	cfg, err := LoadConfig([]string{
		"--hub", "http://localhost:9211",
		"--token", "owt_abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HubURL != "http://localhost:9211" {
		t.Errorf("expected localhost to be allowed, got error")
	}
}

func TestLoadConfig_Insecure(t *testing.T) {
	cfg, err := LoadConfig([]string{
		"--hub", "http://remote.example.com",
		"--token", "owt_abc",
		"--insecure",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Insecure {
		t.Error("expected insecure flag to be set")
	}
}
