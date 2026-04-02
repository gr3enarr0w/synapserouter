package agent

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestExtractCacheKey_StopWords(t *testing.T) {
	k1 := ExtractCacheKey("fix the tests")
	k2 := ExtractCacheKey("fix tests")
	if k1 != k2 {
		t.Errorf("stop words should be removed: %q != %q", k1, k2)
	}
}

func TestExtractCacheKey_Deterministic(t *testing.T) {
	k1 := ExtractCacheKey("implement the login feature with OAuth")
	k2 := ExtractCacheKey("implement the login feature with OAuth")
	if k1 != k2 {
		t.Errorf("same input should produce same key: %q != %q", k1, k2)
	}
}

func TestExtractCacheKey_OrderIndependent(t *testing.T) {
	k1 := ExtractCacheKey("fix broken tests")
	k2 := ExtractCacheKey("broken tests fix")
	if k1 != k2 {
		t.Errorf("word order should not matter: %q != %q", k1, k2)
	}
}

func TestExtractCacheKey_DifferentTasks(t *testing.T) {
	k1 := ExtractCacheKey("fix the tests")
	k2 := ExtractCacheKey("add authentication endpoint")
	if k1 == k2 {
		t.Error("different tasks should produce different keys")
	}
}

func TestPlanCache_StoreAndLookup(t *testing.T) {
	db := newTestDB(t)
	pc := NewPlanCache(db)
	if pc == nil {
		t.Fatal("expected non-nil PlanCache")
	}

	key := ExtractCacheKey("fix the tests")
	err := pc.Store(key, "auto", "fix the tests", "1. Run go test\n2. Fix failures")
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	cached, err := pc.Lookup(key, "auto")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if cached == nil {
		t.Fatal("expected cache hit")
	}

	if cached.OriginalRequest != "fix the tests" {
		t.Errorf("request = %q, want 'fix the tests'", cached.OriginalRequest)
	}
	if cached.AcceptanceCriteria != "1. Run go test\n2. Fix failures" {
		t.Errorf("criteria = %q", cached.AcceptanceCriteria)
	}
	if cached.HitCount != 0 {
		t.Errorf("initial hit count = %d, want 0", cached.HitCount)
	}
}

func TestPlanCache_ModelInvalidation(t *testing.T) {
	db := newTestDB(t)
	pc := NewPlanCache(db)

	key := ExtractCacheKey("fix the tests")
	pc.Store(key, "model-a", "fix the tests", "plan A")

	// Same key, different model → miss
	cached, err := pc.Lookup(key, "model-b")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if cached != nil {
		t.Error("different model should be a cache miss")
	}

	// Same key, same model → hit
	cached, err = pc.Lookup(key, "model-a")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if cached == nil {
		t.Error("same model should be a cache hit")
	}
}

func TestPlanCache_HitCount(t *testing.T) {
	db := newTestDB(t)
	pc := NewPlanCache(db)

	key := ExtractCacheKey("fix tests")
	pc.Store(key, "auto", "fix tests", "plan")

	cached, _ := pc.Lookup(key, "auto")
	if cached.HitCount != 0 {
		t.Errorf("initial hit count = %d", cached.HitCount)
	}

	pc.RecordHit(cached.ID)
	pc.RecordHit(cached.ID)

	cached, _ = pc.Lookup(key, "auto")
	if cached.HitCount != 2 {
		t.Errorf("hit count after 2 RecordHit = %d, want 2", cached.HitCount)
	}
}

func TestPlanCache_NilDB(t *testing.T) {
	pc := NewPlanCache(nil)
	if pc != nil {
		t.Error("nil db should return nil PlanCache")
	}

	// Operations on nil should not panic
	var nilPC *PlanCache
	err := nilPC.Store("key", "model", "req", "criteria")
	if err != nil {
		t.Errorf("nil store should return nil error, got: %v", err)
	}

	cached, err := nilPC.Lookup("key", "model")
	if cached != nil || err != nil {
		t.Error("nil lookup should return nil, nil")
	}

	nilPC.RecordHit(1) // should not panic
}

func TestPlanCache_Upsert(t *testing.T) {
	db := newTestDB(t)
	pc := NewPlanCache(db)

	key := ExtractCacheKey("fix tests")
	pc.Store(key, "auto", "fix tests", "old plan")
	pc.Store(key, "auto", "fix tests v2", "new plan")

	cached, _ := pc.Lookup(key, "auto")
	if cached.AcceptanceCriteria != "new plan" {
		t.Errorf("upsert should update criteria: got %q", cached.AcceptanceCriteria)
	}
	if cached.OriginalRequest != "fix tests v2" {
		t.Errorf("upsert should update request: got %q", cached.OriginalRequest)
	}
}
