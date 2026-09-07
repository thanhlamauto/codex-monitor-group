package queue

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/thanhlamauto/codex-monitor-group/agent/codex-guard/internal/atomicfile"
)

type Item struct {
	Endpoint  string          `json:"endpoint"`
	CreatedAt time.Time       `json:"created_at"`
	Envelope  json.RawMessage `json:"envelope"`
}

type Queue struct {
	path      string
	retention time.Duration
	mu        sync.Mutex
}

func New(path string, retentionDays int) *Queue {
	return &Queue{path: path, retention: time.Duration(retentionDays) * 24 * time.Hour}
}

func (q *Queue) load() ([]Item, error) {
	data, err := os.ReadFile(q.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var items []Item
	if len(data) > 0 {
		err = json.Unmarshal(data, &items)
	}
	return items, err
}

func (q *Queue) save(items []Item) error {
	if err := os.MkdirAll(filepath.Dir(q.path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	tmp := q.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return atomicfile.Replace(tmp, q.path)
}

func (q *Queue) Push(item Item) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	items, err := q.load()
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-q.retention)
	kept := items[:0]
	for _, old := range items {
		if old.CreatedAt.After(cutoff) {
			kept = append(kept, old)
		}
	}
	kept = append(kept, item)
	return q.save(kept)
}
func (q *Queue) Peek() (*Item, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	items, err := q.load()
	if err != nil || len(items) == 0 {
		return nil, err
	}
	return &items[0], nil
}
func (q *Queue) Pop() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	items, err := q.load()
	if err != nil || len(items) == 0 {
		return err
	}
	return q.save(items[1:])
}
func (q *Queue) Clear() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.save(nil)
}
func (q *Queue) Len() int { q.mu.Lock(); defer q.mu.Unlock(); items, _ := q.load(); return len(items) }
