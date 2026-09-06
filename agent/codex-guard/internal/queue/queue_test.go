package queue

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestQueueReplayOrder(t *testing.T) {
	q := New(filepath.Join(t.TempDir(), "queue.json"), 30)
	for _, endpoint := range []string{"/one", "/two"} {
		if err := q.Push(Item{Endpoint: endpoint, CreatedAt: time.Now(), Envelope: json.RawMessage(`{}`)}); err != nil {
			t.Fatal(err)
		}
	}
	item, _ := q.Peek()
	if item.Endpoint != "/one" {
		t.Fatal(item.Endpoint)
	}
	_ = q.Pop()
	item, _ = q.Peek()
	if item.Endpoint != "/two" {
		t.Fatal(item.Endpoint)
	}
}
