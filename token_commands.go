package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/config"
	"github.com/onllm-dev/onwatch/v2/internal/hub"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func runTokenCommand() error {
	args := os.Args[1:]

	tokenIdx := -1
	for i, arg := range args {
		if arg == "token" {
			tokenIdx = i
			break
		}
	}
	if tokenIdx == -1 || len(args) <= tokenIdx+1 {
		return printTokenHelp()
	}

	subCmd := args[tokenIdx+1]
	subArgs := args[tokenIdx+2:]

	switch subCmd {
	case "create":
		return tokenCreate(subArgs)
	case "list":
		return tokenList()
	case "revoke":
		return tokenRevoke(subArgs)
	default:
		return printTokenHelp()
	}
}

func tokenCreate(args []string) error {
	var name, owner string
	var expiresDuration time.Duration

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 < len(args) {
				i++
				name = args[i]
			}
		case "--owner":
			if i+1 < len(args) {
				i++
				owner = args[i]
			}
		case "--expires":
			if i+1 < len(args) {
				i++
				d, err := parseDuration(args[i])
				if err != nil {
					return fmt.Errorf("invalid --expires value %q: %w", args[i], err)
				}
				expiresDuration = d
			}
		default:
			if name == "" {
				name = args[i]
			}
		}
	}

	if name == "" {
		return fmt.Errorf("usage: onwatch token create --name <name> [--owner <owner>] [--expires <duration>]")
	}

	db, err := openTokenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	raw, err := hub.GenerateRawToken()
	if err != nil {
		return fmt.Errorf("failed to generate token: %w", err)
	}

	tokenHash := hub.HashToken(raw)
	prefix := hub.TokenDisplayPrefix(raw)

	var expiresAt *time.Time
	if expiresDuration > 0 {
		t := time.Now().Add(expiresDuration)
		expiresAt = &t
	}

	_, err = db.CreateAgentToken(tokenHash, prefix, name, owner, "sync", expiresAt)
	if err != nil {
		return fmt.Errorf("failed to create token: %w", err)
	}

	fmt.Println("Agent token created successfully.")
	fmt.Println()
	fmt.Printf("  Token:   %s\n", raw)
	fmt.Printf("  Name:    %s\n", name)
	if owner != "" {
		fmt.Printf("  Owner:   %s\n", owner)
	}
	if expiresAt != nil {
		fmt.Printf("  Expires: %s\n", expiresAt.Format(time.RFC3339))
	}
	fmt.Println()
	fmt.Println("Store this token securely - it cannot be retrieved again.")
	return nil
}

func tokenList() error {
	db, err := openTokenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	tokens, err := db.ListAgentTokens()
	if err != nil {
		return fmt.Errorf("failed to list tokens: %w", err)
	}

	if len(tokens) == 0 {
		fmt.Println("No agent tokens found. Create one with: onwatch token create --name <name>")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tOWNER\tPREFIX\tCREATED\tLAST USED\tSTATUS")
	for _, t := range tokens {
		status := "active"
		if t.RevokedAt != nil {
			status = "revoked"
		} else if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
			status = "expired"
		}

		lastUsed := "never"
		if t.LastUsedAt != nil {
			lastUsed = relativeTime(*t.LastUsedAt)
		}

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			t.ID, t.Name, t.Owner, t.Prefix,
			t.CreatedAt.Format("2006-01-02"),
			lastUsed, status)
	}
	return w.Flush()
}

func tokenRevoke(args []string) error {
	var id int64
	var name string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--id":
			if i+1 < len(args) {
				i++
				parsed, err := strconv.ParseInt(args[i], 10, 64)
				if err != nil {
					return fmt.Errorf("invalid --id value: %w", err)
				}
				id = parsed
			}
		case "--name":
			if i+1 < len(args) {
				i++
				name = args[i]
			}
		}
	}

	if id == 0 && name == "" {
		return fmt.Errorf("usage: onwatch token revoke --id <id> or --name <name>")
	}

	db, err := openTokenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if id != 0 {
		if err := db.RevokeAgentToken(id); err != nil {
			return err
		}
		fmt.Printf("Revoked token ID %d.\n", id)
	} else {
		if err := db.RevokeAgentTokenByName(name); err != nil {
			return err
		}
		fmt.Printf("Revoked token %q.\n", name)
	}
	return nil
}

func openTokenDB() (*store.Store, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	db, err := store.New(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database at %s: %w", cfg.DBPath, err)
	}
	return db, nil
}

func printTokenHelp() error {
	fmt.Println("Usage: onwatch token <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  create  Create a new agent token")
	fmt.Println("  list    List all agent tokens")
	fmt.Println("  revoke  Revoke an agent token")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  onwatch token create --name prakersh-macbook")
	fmt.Println("  onwatch token create --name ci-runner --owner ops --expires 90d")
	fmt.Println("  onwatch token list")
	fmt.Println("  onwatch token revoke --name prakersh-macbook")
	fmt.Println("  onwatch token revoke --id 2")
	return nil
}

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0, fmt.Errorf("empty duration")
	}

	suffix := s[len(s)-1]
	numStr := s[:len(s)-1]

	switch suffix {
	case 'd', 'D':
		n, err := strconv.Atoi(numStr)
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	case 'h', 'H':
		n, err := strconv.Atoi(numStr)
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * time.Hour, nil
	default:
		return time.ParseDuration(s)
	}
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
