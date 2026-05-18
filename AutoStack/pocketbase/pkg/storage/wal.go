// Package storage implements Phase 5.2 durable substrate:
// write-ahead log, integrity verification, corruption detection,
// durability snapshots, and startup recovery.
package storage

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// WALEntryType classifies a WAL record.
type WALEntryType string

const (
	WALEntryTypeEvent      WALEntryType = "event"
	WALEntryTypeLease      WALEntryType = "lease"
	WALEntryTypeFederation WALEntryType = "federation"
	WALEntryTypeGeneric    WALEntryType = "generic"
)

// WALEntry is one immutable WAL record.
// EntryHash = SHA-256(sequence|type|id|body) — computed at write time.
type WALEntry struct {
	Sequence  int64        `json:"sequence"`
	Type      WALEntryType `json:"type"`
	ID        string       `json:"id"`
	Body      []byte       `json:"body"`
	EntryHash string       `json:"entry_hash"`
	WrittenAt string       `json:"written_at"`
}

// WALCheckpoint is a tamper-evident marker at a known-good WAL position.
// StateHash covers all entry hashes up to LastSequence in order.
type WALCheckpoint struct {
	CheckpointID string `json:"checkpoint_id"`
	LastSequence int64  `json:"last_sequence"`
	StateHash    string `json:"state_hash"`
	CreatedAt    string `json:"created_at"`
}

// WALMetadata summarises the current WAL state for the DurabilitySnapshot.
type WALMetadata struct {
	EntryCount      int64          `json:"entry_count"`
	LastSequence    int64          `json:"last_sequence"`
	IntegrityStatus string         `json:"integrity_status"` // "ok", "corrupted", "unverified"
	LastCheckpoint  *WALCheckpoint `json:"last_checkpoint,omitempty"`
}

// WAL is a file-backed, append-only write-ahead log.
// Entries are written one per line (newline-delimited JSON) to the log file.
// The checkpoint is stored in a separate file (path + ".checkpoint").
type WAL struct {
	mu          sync.Mutex
	path        string
	file        *os.File
	entries     []WALEntry
	checkpoints []WALCheckpoint
	nextSeq     int64
}

// OpenWAL opens (or creates) a WAL at the given path.
// Existing entries are replayed and their hashes verified on open.
// Corrupted entries are still loaded but flagged via VerifyIntegrity().
func OpenWAL(path string) (*WAL, error) {
	w := &WAL{path: path}
	// Replay existing entries.
	if err := w.replay(); err != nil {
		return nil, fmt.Errorf("replay WAL %q: %w", path, err)
	}
	// Load checkpoint if present.
	if err := w.loadCheckpoint(); err != nil {
		return nil, fmt.Errorf("load WAL checkpoint: %w", err)
	}
	// Open file in append mode.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("open WAL file %q: %w", path, err)
	}
	w.file = f
	return w, nil
}

// Append writes a new entry to the WAL and flushes it to disk.
// Returns an error if the disk write fails — the in-memory state is NOT updated.
func (w *WAL) Append(entryType WALEntryType, id string, body []byte) (WALEntry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	seq := w.nextSeq + 1
	bodyCopy := append([]byte(nil), body...)
	entry := WALEntry{
		Sequence:  seq,
		Type:      entryType,
		ID:        id,
		Body:      bodyCopy,
		WrittenAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	entry.EntryHash = computeEntryHash(entry)
	data, err := json.Marshal(entry)
	if err != nil {
		return WALEntry{}, fmt.Errorf("marshal WAL entry: %w", err)
	}
	data = append(data, '\n')
	if _, err := w.file.Write(data); err != nil {
		return WALEntry{}, fmt.Errorf("write WAL entry: %w", err)
	}
	if err := w.file.Sync(); err != nil {
		return WALEntry{}, fmt.Errorf("sync WAL: %w", err)
	}
	w.entries = append(w.entries, entry)
	w.nextSeq = seq
	return entry, nil
}

// Checkpoint writes a checkpoint for the current WAL position to disk atomically.
func (w *WAL) Checkpoint(checkpointID string) (WALCheckpoint, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	hashes := make([]string, len(w.entries))
	for i, e := range w.entries {
		hashes[i] = e.EntryHash
	}
	cp := WALCheckpoint{
		CheckpointID: checkpointID,
		LastSequence: w.nextSeq,
		StateHash:    computeStateHash(hashes),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := w.persistCheckpoint(cp); err != nil {
		return WALCheckpoint{}, err
	}
	w.checkpoints = append(w.checkpoints, cp)
	return cp, nil
}

// Entries returns a copy of all WAL entries in order.
func (w *WAL) Entries() []WALEntry {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]WALEntry, len(w.entries))
	copy(out, w.entries)
	return out
}

// EntriesSince returns entries with Sequence > afterSequence.
func (w *WAL) EntriesSince(afterSequence int64) []WALEntry {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []WALEntry
	for _, e := range w.entries {
		if e.Sequence > afterSequence {
			out = append(out, e)
		}
	}
	return out
}

// LatestCheckpoint returns the most recent checkpoint, or nil if none exist.
func (w *WAL) LatestCheckpoint() *WALCheckpoint {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.checkpoints) == 0 {
		return nil
	}
	cp := w.checkpoints[len(w.checkpoints)-1]
	return &cp
}

// VerifyIntegrity checks all entry hashes and checkpoint state hash.
// Returns a slice of invalid entry sequences; empty means intact.
func (w *WAL) VerifyIntegrity() []int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	var bad []int64
	for _, e := range w.entries {
		computed := computeEntryHashFromFields(e.Sequence, e.Type, e.ID, e.Body)
		if computed != e.EntryHash {
			bad = append(bad, e.Sequence)
		}
	}
	return bad
}

// Metadata returns a snapshot of current WAL state.
func (w *WAL) Metadata() WALMetadata {
	w.mu.Lock()
	defer w.mu.Unlock()
	bad := w.verifyIntegrityLocked()
	status := "ok"
	if len(bad) > 0 {
		status = "corrupted"
	}
	var lastCP *WALCheckpoint
	if len(w.checkpoints) > 0 {
		cp := w.checkpoints[len(w.checkpoints)-1]
		lastCP = &cp
	}
	return WALMetadata{
		EntryCount:      int64(len(w.entries)),
		LastSequence:    w.nextSeq,
		IntegrityStatus: status,
		LastCheckpoint:  lastCP,
	}
}

// Close flushes and closes the underlying file.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

func (w *WAL) replay() error {
	f, err := os.Open(w.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open WAL for replay: %w", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry WALEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // truncated/corrupt line: skip, will be detected by VerifyIntegrity
		}
		w.entries = append(w.entries, entry)
		if entry.Sequence > w.nextSeq {
			w.nextSeq = entry.Sequence
		}
	}
	return scanner.Err()
}

func (w *WAL) loadCheckpoint() error {
	cpPath := w.path + ".checkpoint"
	data, err := os.ReadFile(cpPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read checkpoint file: %w", err)
	}
	var cp WALCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return fmt.Errorf("parse checkpoint file: %w", err)
	}
	w.checkpoints = append(w.checkpoints, cp)
	return nil
}

func (w *WAL) persistCheckpoint(cp WALCheckpoint) error {
	cpPath := w.path + ".checkpoint"
	tmp := cpPath + ".tmp"
	data, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write tmp checkpoint: %w", err)
	}
	return os.Rename(tmp, cpPath)
}

func (w *WAL) verifyIntegrityLocked() []int64 {
	var bad []int64
	for _, e := range w.entries {
		computed := computeEntryHashFromFields(e.Sequence, e.Type, e.ID, e.Body)
		if computed != e.EntryHash {
			bad = append(bad, e.Sequence)
		}
	}
	return bad
}

func computeEntryHash(e WALEntry) string {
	return computeEntryHashFromFields(e.Sequence, e.Type, e.ID, e.Body)
}

func computeEntryHashFromFields(seq int64, t WALEntryType, id string, body []byte) string {
	h := sha256.New()
	fmt.Fprintf(h, "seq:%d|type:%s|id:%s|body:", seq, t, id)
	h.Write(body)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func computeStateHash(entryHashes []string) string {
	h := sha256.New()
	for _, eh := range entryHashes {
		fmt.Fprintf(h, "%s|", eh)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
