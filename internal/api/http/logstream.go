package http

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// LogEntry represents a single log entry in the ring buffer.
type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// LogRingBuffer is a fixed-size ring buffer that stores the most recent
// log entries in memory. It is safe for concurrent use.
type LogRingBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	size    int
	pos     int // next write position
	full    bool
}

// NewLogRingBuffer creates a new ring buffer with the given capacity.
// A non-positive size is clamped to 1 so Add never divides by zero.
func NewLogRingBuffer(size int) *LogRingBuffer {
	if size < 1 {
		size = 1
	}
	return &LogRingBuffer{
		entries: make([]LogEntry, size),
		size:    size,
	}
}

// Add appends a log entry to the ring buffer, overwriting the oldest
// entry when the buffer is full.
func (b *LogRingBuffer) Add(level, message string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries[b.pos] = LogEntry{
		Time:    time.Now().UTC(),
		Level:   level,
		Message: message,
	}
	b.pos = (b.pos + 1) % b.size
	if b.pos == 0 {
		b.full = true
	}
}

// Entries returns all stored log entries in chronological order
// (oldest first).
func (b *LogRingBuffer) Entries() []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.full && b.pos == 0 {
		return []LogEntry{} // empty buffer
	}

	var result []LogEntry
	if b.full {
		// Read from pos (oldest) around to pos-1 (newest)
		for i := 0; i < b.size; i++ {
			idx := (b.pos + i) % b.size
			result = append(result, b.entries[idx])
		}
	} else {
		result = make([]LogEntry, b.pos)
		copy(result, b.entries[:b.pos])
	}
	return result
}

// GlobalLogBuffer is the shared ring buffer used by the log capture
// hook and the /api/logs endpoint.
var GlobalLogBuffer = NewLogRingBuffer(50)

// LogsGet returns the last 50 log entries as JSON.
func (h *Handler) LogsGet(c *gin.Context) {
	entries := GlobalLogBuffer.Entries()
	c.JSON(http.StatusOK, gin.H{"logs": entries})
}
