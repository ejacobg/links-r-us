package elasticsearch

import (
	"fmt"
	"github.com/ejacobg/links-r-us/index/indextest"
	"os"
	"strings"
	"testing"
)

// TestAcceptance runs the acceptance tests provided by the indextest.Suite type.
func TestAcceptance(t *testing.T) {
	nodeList := os.Getenv("ES_NODES")
	if nodeList == "" {
		t.Skip("Missing ES_NODES env var; skipping elasticsearch-backed index test suite")
	}

	fmt.Println(nodeList)

	idx, err := NewIndexer(strings.Split(nodeList, ","), true)
	if err != nil {
		t.Fatalf("failed to create indexer: %v", err)
	}

	suite := indextest.Suite{
		Idx: idx,
		BeforeEach: func(t *testing.T) {
			if idx.es != nil {
				_, err = idx.es.Indices.Delete([]string{indexName})
				if err != nil {
					t.Fatalf("failed to delete index: %v", err)
				}
				err = ensureIndex(idx.es)
				if err != nil {
					t.Fatalf("failed to create index: %v", err)
				}
			}
		},
	}

	suite.TestIndexer(t)
}
