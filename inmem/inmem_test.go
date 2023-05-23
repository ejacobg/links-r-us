package inmem

import (
	"github.com/ejacobg/links-r-us/graph/graphtest"
	"testing"
)

func TestAcceptance(t *testing.T) {
	suite := graphtest.Suite{}

	suite.BeforeEach = func(_ *testing.T) {
		suite.G = NewGraph()
	}

	suite.TestGraph(t)
}

// Writing individual tests for debugging purposes.

func TestUpsertEdge(t *testing.T) {
	graphtest.TestUpsertEdge(t, NewGraph())
}

func TestRemoveStaleEdges(t *testing.T) {
	graphtest.TestRemoveStaleEdges(t, NewGraph())
}

func TestEdgeIteratorTimeFilter(t *testing.T) {
	graphtest.TestEdgeIteratorTimeFilter(t, NewGraph())
}

func TestLinkIteratorTimeFilter(t *testing.T) {
	graphtest.TestLinkIteratorTimeFilter(t, NewGraph())
}

func TestConcurrentEdgeIterators(t *testing.T) {
	graphtest.TestConcurrentEdgeIterators(t, NewGraph())
}
