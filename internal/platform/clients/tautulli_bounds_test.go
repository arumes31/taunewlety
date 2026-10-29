package clients

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetRecentlyAdded_TruncatesToRequestedCount(t *testing.T) {
	items := make([]map[string]any, 500)
	for i := range items {
		items[i] = map[string]any{"title": fmt.Sprintf("M%d", i), "rating_key": i + 1}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{"result": "success", "data": map[string]any{"recently_added": items}},
		})
	}))
	defer srv.Close()

	got, err := NewTautulliClient(srv.URL, "k").GetRecentlyAdded(100)
	if err != nil {
		t.Fatalf("GetRecentlyAdded: %v", err)
	}
	if len(got) != 100 {
		t.Errorf("expected response truncated to 100 items, got %d", len(got))
	}
}

func TestGetWatchHistoryBatch_DedupesAndBoundsConcurrency(t *testing.T) {
	var mu sync.Mutex
	perKey := map[string]int{}
	var inFlight, peak, total int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&inFlight, 1)
		defer atomic.AddInt32(&inFlight, -1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		atomic.AddInt32(&total, 1)
		mu.Lock()
		perKey[r.URL.Query().Get("rating_key")]++
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{"result": "success", "data": map[string]any{"recordsFiltered": 1}},
		})
	}))
	defer srv.Close()

	// 2000 keys, but only 300 distinct values.
	keys := make([]string, 0, 2000)
	for i := 0; i < 1000; i++ {
		keys = append(keys, fmt.Sprintf("%d", i%300), "1")
	}

	res, err := NewTautulliClient(srv.URL, "k").GetWatchHistoryBatch(keys)
	if err != nil {
		t.Fatalf("GetWatchHistoryBatch: %v", err)
	}
	if total > maxWatchHistoryBatch {
		t.Errorf("expected at most %d requests, got %d", maxWatchHistoryBatch, total)
	}
	if peak > maxWatchHistoryWorkers {
		t.Errorf("expected at most %d concurrent requests, got %d", maxWatchHistoryWorkers, peak)
	}
	for k, n := range perKey {
		if n > 1 {
			t.Errorf("key %s requested %d times", k, n)
		}
	}
	if len(res) != int(total) {
		t.Errorf("expected %d results, got %d", total, len(res))
	}
}
