package bleve

import (
	"github.com/ejacobg/links-r-us/index/indextest"
	"testing"
)

func TestAcceptance(t *testing.T) {
	suite := indextest.Suite{}

	suite.BeforeEach = func(t *testing.T) {
		idx, err := NewIndexer()
		if err != nil {
			t.Fatalf("failed to create indexer: %v", err)
		}
		suite.Idx = idx
	}

	suite.AfterEach = func(t *testing.T) {
		idx := suite.Idx.(*Indexer)
		if err := idx.Close(); err != nil {
			t.Fatalf("failed to close indexer: %v", err)
		}
	}

	suite.TestIndexer(t)
}
