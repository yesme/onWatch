package agentclient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/hub"
)

const (
	maxQueueSize    = 10 << 20 // 10 MB
	maxQueueAgeDays = 7
)

// Queue buffers sync payloads to disk when the hub is unreachable.
type Queue struct {
	dir string
}

// NewQueue creates a disk-backed queue at the given directory.
func NewQueue(dir string) (*Queue, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("agentclient.NewQueue: %w", err)
	}
	return &Queue{dir: dir}, nil
}

// Enqueue appends a sync request to the queue file for today.
func (q *Queue) Enqueue(req *hub.SyncRequest) error {
	q.evictOld()

	filename := time.Now().UTC().Format("2006-01-02") + ".jsonl"
	path := filepath.Join(q.dir, filename)

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("agentclient.Queue.Enqueue: marshal: %w", err)
	}

	if q.totalSize()+int64(len(data)+1) > maxQueueSize {
		return fmt.Errorf("agentclient.Queue.Enqueue: queue full (%d bytes)", maxQueueSize)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("agentclient.Queue.Enqueue: open: %w", err)
	}
	defer f.Close()

	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("agentclient.Queue.Enqueue: write: %w", err)
	}
	return nil
}

// Drain reads up to limit queued requests, oldest first. Returns the requests
// and calls remove() to delete the consumed file entries on success.
func (q *Queue) Drain(limit int) ([]hub.SyncRequest, func(), error) {
	files, err := q.sortedFiles()
	if err != nil || len(files) == 0 {
		return nil, func() {}, err
	}

	var results []hub.SyncRequest
	var consumed []string

	for _, name := range files {
		if len(results) >= limit {
			break
		}
		path := filepath.Join(q.dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var req hub.SyncRequest
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				continue
			}
			results = append(results, req)
			if len(results) >= limit {
				break
			}
		}
		consumed = append(consumed, path)
	}

	remove := func() {
		for _, p := range consumed {
			os.Remove(p)
		}
	}

	return results, remove, nil
}

// IsEmpty returns true if the queue has no entries.
func (q *Queue) IsEmpty() bool {
	files, _ := q.sortedFiles()
	return len(files) == 0
}

func (q *Queue) sortedFiles() ([]string, error) {
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func (q *Queue) totalSize() int64 {
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		return 0
	}
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err == nil {
			total += info.Size()
		}
	}
	return total
}

func (q *Queue) evictOld() {
	cutoff := time.Now().UTC().AddDate(0, 0, -maxQueueAgeDays).Format("2006-01-02")
	entries, err := os.ReadDir(q.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".jsonl") && name < cutoff {
			os.Remove(filepath.Join(q.dir, name))
		}
	}
}
