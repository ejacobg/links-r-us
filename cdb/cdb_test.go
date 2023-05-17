package cdb

import (
	"database/sql"
	"github.com/ejacobg/links-r-us/graph/graphtest"
	"os"
	"testing"
)

func TestAcceptance(t *testing.T) {
	// Note: DSN should use postgresql:// scheme, not cockroachdb://.
	dsn := os.Getenv("CDB_DSN")
	if dsn == "" {
		t.Skip("Missing CDB_DSN env var; skipping cockroachdb-backed graph test suite")
	}

	g, err := NewGraph(dsn)
	if err != nil {
		t.Fatalf("failed to create graph: %v", err)
	}

	suite := graphtest.Suite{
		G: g,
		BeforeEach: func(t *testing.T) {
			flushDB(t, g.db)
		},
	}

	suite.TestGraph(t)

	if g.db != nil {
		flushDB(t, g.db)
		if err = g.db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	}
}

func flushDB(t *testing.T, db *sql.DB) {
	_, err := db.Exec("DELETE FROM links")
	if err != nil {
		t.Fatalf("failed to delete links: %v", err)
	}
	_, err = db.Exec("DELETE FROM edges")
	if err != nil {
		t.Fatalf("failed to delete edges: %v", err)
	}
}
