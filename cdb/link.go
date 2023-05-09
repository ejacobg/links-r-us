package cdb

import (
	"database/sql"
	"fmt"
	"github.com/ejacobg/links-r-us/graph"
)

// linkIterator is a graph.LinkIterator implementation for the cdb graph.
type linkIterator struct {
	rows        *sql.Rows
	lastErr     error
	latchedLink *graph.Link
}

// Next implements graph.LinkIterator.
func (i *linkIterator) Next() bool {
	if i.lastErr != nil || !i.rows.Next() {
		return false
	}

	l := new(graph.Link)
	i.lastErr = i.rows.Scan(&l.ID, &l.URL, &l.RetrievedAt)
	if i.lastErr != nil {
		return false
	}
	l.RetrievedAt = l.RetrievedAt.UTC()

	i.latchedLink = l
	return true
}

// Error implements graph.LinkIterator.
func (i *linkIterator) Error() error {
	return i.lastErr
}

// Close implements graph.LinkIterator.
func (i *linkIterator) Close() error {
	err := i.rows.Close()
	if err != nil {
		return fmt.Errorf("link iterator: %w", err)
	}
	return nil
}

// Link implements graph.LinkIterator.
func (i *linkIterator) Link() *graph.Link {
	return i.latchedLink
}
