package graph

import (
	"github.com/google/uuid"
	"time"
)

// Edge describes a graph edge that originates from Src and terminates
// at Dst.
type Edge struct {
	// A unique identifier for the edge.
	ID uuid.UUID

	// The origin link.
	Src uuid.UUID

	// The destination link.
	Dst uuid.UUID

	// The timestamp when the link was last updated.
	UpdatedAt time.Time
}

// EdgeIterator is implemented by objects that can iterate the graph edges.
type EdgeIterator interface {
	Iterator

	// Edge returns the currently fetched edge objects.
	Edge() *Edge
}
