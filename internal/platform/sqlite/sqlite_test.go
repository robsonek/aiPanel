package sqlite

import (
	"context"
	"os"
	"testing"
)

func TestStoreInit_CreatesAllDBFiles(t *testing.T) {
	store := New(t.TempDir())
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}
	for _, p := range []string{store.PanelDB, store.AuditDB, store.QueueDB} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected db file %s: %v", p, err)
		}
	}
}
