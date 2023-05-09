package inmem

import "github.com/ejacobg/links-r-us/graph"

// linkIterator is a graph.LinkIterator implementation for the in-memory graph.
type linkIterator struct {
	g *Graph

	links []*graph.Link
	curr  int
}

// Next implements graph.LinkIterator.
func (i *linkIterator) Next() bool {
	if i.curr >= len(i.links) {
		return false
	}
	i.curr++
	return true
}

// Error implements graph.LinkIterator.
func (i *linkIterator) Error() error {
	return nil
}

// Close implements graph.LinkIterator.
func (i *linkIterator) Close() error {
	return nil
}

// Link implements graph.LinkIterator.
func (i *linkIterator) Link() *graph.Link {
	// The link pointer contents may be overwritten by a graph update; to
	// avoid data-races we acquire the read lock first and clone the link
	i.g.mu.RLock()
	link := new(graph.Link)
	*link = *i.links[i.curr-1]
	i.g.mu.RUnlock()
	return link
}
