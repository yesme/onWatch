package agentclient

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/hub"
)

// CostScanner scans local JSONL session logs for token/cost data.
type CostScanner struct {
	statePath string
	state     scanState
}

type scanState struct {
	Files map[string]fileScanState `json:"files"`
}

type fileScanState struct {
	Offset  int64  `json:"offset"`
	ModTime string `json:"mod_time"`
}

// NewCostScanner creates a scanner that persists state to the given path.
func NewCostScanner(statePath string) *CostScanner {
	cs := &CostScanner{
		statePath: statePath,
		state:     scanState{Files: make(map[string]fileScanState)},
	}
	cs.loadState()
	return cs
}

// Scan discovers and parses JSONL files, returning daily cost buckets.
func (cs *CostScanner) Scan() ([]hub.CostLogPayload, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	buckets := make(map[string]*hub.CostLogPayload) // key: "provider:date"
	modelBuckets := make(map[string]map[string]*hub.CostLogModelBreakdown) // key: "provider:date" -> model

	// Claude Code logs
	claudeDir := filepath.Join(home, ".claude", "projects")
	cs.scanDir(claudeDir, "anthropic", buckets, modelBuckets)

	// Codex logs
	codexDir := filepath.Join(home, ".codex", "sessions")
	cs.scanDir(codexDir, "codex", buckets, modelBuckets)

	cs.saveState()

	var results []hub.CostLogPayload
	for key, bucket := range buckets {
		if mbs, ok := modelBuckets[key]; ok {
			for _, mb := range mbs {
				bucket.ModelBreakdowns = append(bucket.ModelBreakdowns, *mb)
			}
		}
		results = append(results, *bucket)
	}
	return results, nil
}

func (cs *CostScanner) scanDir(dir, provider string, buckets map[string]*hub.CostLogPayload, modelBuckets map[string]map[string]*hub.CostLogModelBreakdown) {
	files, err := findJSONLFiles(dir)
	if err != nil {
		return
	}

	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		prev, exists := cs.state.Files[path]
		modStr := info.ModTime().UTC().Format(time.RFC3339)
		if exists && prev.ModTime == modStr && prev.Offset >= info.Size() {
			continue
		}

		var offset int64
		if exists && prev.ModTime == modStr {
			offset = prev.Offset
		}

		newOffset := cs.scanFile(path, provider, offset, buckets, modelBuckets)
		cs.state.Files[path] = fileScanState{Offset: newOffset, ModTime: modStr}
	}
}

func (cs *CostScanner) scanFile(path, provider string, offset int64, buckets map[string]*hub.CostLogPayload, modelBuckets map[string]map[string]*hub.CostLogModelBreakdown) int64 {
	f, err := os.Open(path)
	if err != nil {
		return offset
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return offset
		}
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)

	seen := make(map[string]bool) // dedup by message ID

	for scanner.Scan() {
		line := scanner.Bytes()
		if !containsUsage(line) {
			continue
		}

		var entry jsonlEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if entry.Type != "assistant" || entry.Message.Usage.InputTokens == 0 && entry.Message.Usage.OutputTokens == 0 {
			continue
		}

		msgID := entry.Message.ID
		if msgID != "" && seen[msgID] {
			continue
		}
		if msgID != "" {
			seen[msgID] = true
		}

		date := "unknown"
		if entry.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
				date = t.Format("2006-01-02")
			} else if t, err := time.Parse("2006-01-02T15:04:05.000Z", entry.Timestamp); err == nil {
				date = t.Format("2006-01-02")
			}
		}

		model := normalizeModel(entry.Message.Model)
		key := provider + ":" + date

		bucket, ok := buckets[key]
		if !ok {
			bucket = &hub.CostLogPayload{
				Provider: provider,
				Date:     date,
			}
			buckets[key] = bucket
			modelBuckets[key] = make(map[string]*hub.CostLogModelBreakdown)
		}

		u := entry.Message.Usage
		bucket.InputTokens += u.InputTokens
		bucket.OutputTokens += u.OutputTokens
		bucket.CacheReadTokens += u.CacheReadInputTokens
		bucket.CacheCreationTokens += u.CacheCreationInputTokens
		bucket.SessionsCount++

		mb, ok := modelBuckets[key][model]
		if !ok {
			mb = &hub.CostLogModelBreakdown{Model: model}
			modelBuckets[key][model] = mb
			bucket.ModelsUsed = appendUnique(bucket.ModelsUsed, model)
		}
		mb.InputTokens += u.InputTokens
		mb.OutputTokens += u.OutputTokens
	}

	pos, _ := f.Seek(0, 1)
	return pos
}

type jsonlEntry struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			InputTokens               int64 `json:"input_tokens"`
			OutputTokens              int64 `json:"output_tokens"`
			CacheReadInputTokens      int64 `json:"cache_read_input_tokens"`
			CacheCreationInputTokens  int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func containsUsage(line []byte) bool {
	return strings.Contains(string(line), `"input_tokens"`)
}

func normalizeModel(model string) string {
	if model == "" {
		return "unknown"
	}
	parts := strings.Split(model, "-")
	// Strip date suffix like "claude-opus-4-7-20250514" -> "claude-opus-4-7"
	if len(parts) > 2 {
		last := parts[len(parts)-1]
		if len(last) == 8 && last[0] >= '2' {
			return strings.Join(parts[:len(parts)-1], "-")
		}
	}
	return model
}

func findJSONLFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func appendUnique(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}

func (cs *CostScanner) loadState() {
	data, err := os.ReadFile(cs.statePath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &cs.state)
	if cs.state.Files == nil {
		cs.state.Files = make(map[string]fileScanState)
	}
}

func (cs *CostScanner) saveState() {
	data, err := json.Marshal(cs.state)
	if err != nil {
		return
	}
	dir := filepath.Dir(cs.statePath)
	os.MkdirAll(dir, 0700)
	os.WriteFile(cs.statePath, data, 0600)
}

// Reset clears scanner state (useful for testing).
func (cs *CostScanner) Reset() {
	cs.state = scanState{Files: make(map[string]fileScanState)}
	os.Remove(cs.statePath)
}

// EstimateCost estimates USD cost for a model given token counts.
// Placeholder - actual pricing can be updated as needed.
func EstimateCost(model string, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int64) float64 {
	// Simplified pricing per million tokens (approximate)
	type pricing struct{ input, output float64 }
	prices := map[string]pricing{
		"claude-opus-4-7":   {15.0, 75.0},
		"claude-opus-4-6":   {15.0, 75.0},
		"claude-sonnet-4-6": {3.0, 15.0},
		"claude-haiku-4-5":  {0.80, 4.0},
	}

	p, ok := prices[model]
	if !ok {
		p = pricing{3.0, 15.0} // default to sonnet-class pricing
	}

	inputCost := float64(inputTokens) * p.input / 1_000_000
	outputCost := float64(outputTokens) * p.output / 1_000_000
	cacheReadCost := float64(cacheReadTokens) * p.input * 0.1 / 1_000_000
	cacheCreateCost := float64(cacheCreationTokens) * p.input * 1.25 / 1_000_000

	return inputCost + outputCost + cacheReadCost + cacheCreateCost
}

// FmtCost formats a USD amount for display.
func FmtCost(usd float64) string {
	return fmt.Sprintf("$%.2f", usd)
}
