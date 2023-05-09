package inmem

import "github.com/ejacobg/links-r-us/graph"

// edgeIterator is a graph.EdgeIterator implementation for the in-memory graph.
type edgeIterator struct {
	g *Graph

	edges []*graph.Edge
	curr  int
}

// Next implements graph.LinkIterator.
func (i *edgeIterator) Next() bool {
	if i.curr >= len(i.edges) {
		return false
	}
	i.curr++
	return true
}

// Error implements graph.LinkIterator.
func (i *edgeIterator) Error() error {
	return nil
}

// Close implements graph.LinkIterator.
func (i *edgeIterator) Close() error {
	return nil
}

// Edge implements graph.LinkIterator.
func (i *edgeIterator) Edge() *graph.Edge {
	// The edge pointer contents may be overwritten by a graph update; to
	// avoid data-races we acquire the read lock first and clone the edge
	i.g.mu.RLock()
	edge := new(graph.Edge)
	*edge = *i.edges[i.curr-1]
	i.g.mu.RUnlock()
	return edge
}
