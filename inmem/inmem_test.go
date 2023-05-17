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
