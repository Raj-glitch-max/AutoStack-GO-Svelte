package events

import (
	"fmt"
	"os"
	"strconv"
	"sync"
)

// LogicalClockAllocator is a monotonic, crash-safe logical clock backed by a file.
// On each Next() call the new value is written to disk (via atomic rename) before
// being returned. If the process crashes between write and return, the persisted
// value is already advanced — the next open picks up from there.
//
// Guarantees (single-process only):
//   - Strictly monotonic within one process (mutex-protected).
//   - Values persist across restarts: reopening the same path resumes from the
//     last persisted value.
//   - Atomic writes via rename prevent partial-write corruption.
//
// NOT claimed: distributed clock correctness or multi-process safety.
type LogicalClockAllocator struct {
	mu      sync.Mutex
	path    string
	current int64
}

// NewLogicalClockAllocator opens (or creates) a clock file at path and returns a ready allocator.
// If the file exists, the allocator resumes from the stored value.
func NewLogicalClockAllocator(path string) (*LogicalClockAllocator, error) {
	var current int64
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read clock file %q: %w", path, err)
	}
	if len(data) > 0 {
		parsed, err := strconv.ParseInt(string(data), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse clock file %q: %w", path, err)
		}
		current = parsed
	}
	return &LogicalClockAllocator{path: path, current: current}, nil
}

// Next returns the next strictly monotonic clock value and persists it to disk.
// Returns an error only if the disk write fails; in that case the in-memory counter
// is NOT advanced (the caller should retry or treat it as a fatal condition).
func (a *LogicalClockAllocator) Next() (int64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	next := a.current + 1
	if err := a.persist(next); err != nil {
		return 0, fmt.Errorf("persist clock %d: %w", next, err)
	}
	a.current = next
	return next, nil
}

// Current returns the last allocated value without advancing the clock.
func (a *LogicalClockAllocator) Current() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.current
}

func (a *LogicalClockAllocator) persist(value int64) error {
	tmp := a.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.FormatInt(value, 10)), 0600); err != nil {
		return fmt.Errorf("write tmp clock file: %w", err)
	}
	if err := os.Rename(tmp, a.path); err != nil {
		return fmt.Errorf("rename clock file: %w", err)
	}
	return nil
}
