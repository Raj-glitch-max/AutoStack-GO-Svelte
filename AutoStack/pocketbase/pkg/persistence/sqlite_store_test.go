package persistence

import (
	"testing"

	"github.com/janlauber/one-click/pkg/executor"
)

func openTestSQLite(t *testing.T) *SQLiteKVStore {
	t.Helper()
	s, err := NewSQLiteKVStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteKVStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSQLiteKVStore_PutGet(t *testing.T) {
	s := openTestSQLite(t)
	if err := s.Put(hctx, "k1", []byte("value1")); err != nil {
		t.Fatalf("put: %v", err)
	}
	v, found, err := s.Get(hctx, "k1")
	if err != nil || !found || string(v) != "value1" {
		t.Fatalf("get: v=%q found=%v err=%v", v, found, err)
	}
}

func TestSQLiteKVStore_DuplicatePut_Rejected(t *testing.T) {
	s := openTestSQLite(t)
	_ = s.Put(hctx, "dup", []byte("v1"))
	if err := s.Put(hctx, "dup", []byte("v2")); err == nil {
		t.Fatal("duplicate put must be rejected")
	}
}

func TestSQLiteKVStore_Get_NotFound(t *testing.T) {
	s := openTestSQLite(t)
	v, found, err := s.Get(hctx, "missing")
	if err != nil || found || v != nil {
		t.Fatalf("expected not-found: v=%v found=%v err=%v", v, found, err)
	}
}

func TestSQLiteKVStore_List_Prefix(t *testing.T) {
	s := openTestSQLite(t)
	_ = s.Put(hctx, "task:exec-1:t-1", []byte("a"))
	_ = s.Put(hctx, "task:exec-1:t-2", []byte("b"))
	_ = s.Put(hctx, "worker:w-1", []byte("c"))

	vals, err := s.List(hctx, "task:exec-1:")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(vals) != 2 {
		t.Fatalf("expected 2 task entries, got %d", len(vals))
	}
}

func TestSQLiteKVStore_DeepCopy(t *testing.T) {
	s := openTestSQLite(t)
	_ = s.Put(hctx, "copy-key", []byte("hello"))
	v, _, _ := s.Get(hctx, "copy-key")
	v[0] = 'X'
	v2, _, _ := s.Get(hctx, "copy-key")
	if v2[0] == 'X' {
		t.Fatal("get must return a deep copy")
	}
}

func TestSQLiteKVStore_BacksExecutionTaskStore(t *testing.T) {
	s := openTestSQLite(t)
	taskStore := NewExecutionTaskStore(s)

	task := executor.NewExecutionTask("t-sql-1", "exec-sql-1", "wf-1", "step-1", "tenant-1", executor.TaskTypeDeploy, 1, 1)
	if err := taskStore.Save(hctx, task); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, found, err := taskStore.Load(hctx, "t-sql-1")
	if err != nil || !found {
		t.Fatalf("load: found=%v err=%v", found, err)
	}
	if loaded.TaskID != task.TaskID {
		t.Fatalf("task ID mismatch: %s vs %s", loaded.TaskID, task.TaskID)
	}
}

func TestSQLiteKVStore_Size(t *testing.T) {
	s := openTestSQLite(t)
	_ = s.Put(hctx, "a", []byte("1"))
	_ = s.Put(hctx, "b", []byte("2"))
	n, err := s.Size()
	if err != nil || n != 2 {
		t.Fatalf("expected size 2, got %d err=%v", n, err)
	}
}
