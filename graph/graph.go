package graph

import (
	"github.com/google/uuid"
	"time"
)

// Because multiple services may be querying the graph, you may not receive a "live" copy of a link or page.
// If this is the case, why don't we just return a normal Link or Edge, rather than a *Link or *Edge?

// Graph is implemented by objects that can mutate or query a link graph.
type Graph interface {
	// UpsertLink creates a new link or updates an existing link.
	UpsertLink(link *Link) error

	// FindLink looks up a link by its ID.
	FindLink(id uuid.UUID) (*Link, error)

	// Links returns an iterator for the set of links whose IDs belong to the
	// [fromID, toID) range and were retrieved before the provided timestamp.
	Links(fromID, toID uuid.UUID, retrievedBefore time.Time) (LinkIterator, error)

	// UpsertEdge creates a new edge or updates an existing edge.
	UpsertEdge(edge *Edge) error

	// Edges returns an iterator for the set of edges whose source vertex IDs
	// belong to the [fromID, toID) range and were updated before the provided
	// timestamp.
	Edges(fromID, toID uuid.UUID, updatedBefore time.Time) (EdgeIterator, error)

	// RemoveStaleEdges removes any edge that originates from the specified
	// link ID and was updated before the specified timestamp.
	RemoveStaleEdges(fromID uuid.UUID, updatedBefore time.Time) error
}

// Iterator is implemented by graph objects that can be iterated.
type Iterator interface {
	// Next advances the iterator. If no more items are available or an
	// error occurs, calls to Next() return false.
	Next() bool

	// Error returns the last error encountered by the iterator.
	Error() error

	// Close releases any resources associated with an iterator.
	Close() error
}
