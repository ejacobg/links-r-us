package graph

import (
	"github.com/google/uuid"
	"time"
)

// Link encapsulates all information about a link discovered by the Links 'R'
// Us crawler.
type Link struct {
	// A unique identifier for the link.
	ID uuid.UUID

	// The link target.
	URL string

	// The timestamp when the link was last retrieved.
	RetrievedAt time.Time
}

// LinkIterator is implemented by objects that can iterate the graph links.
type LinkIterator interface {
	Iterator

	// Link returns the currently fetched link object.
	Link() *Link
}
