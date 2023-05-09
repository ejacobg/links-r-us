package cdb

import (
	"database/sql"
	"fmt"
	"github.com/ejacobg/links-r-us/graph"
)

// edgeIterator is a graph.EdgeIterator implementation for the cdb graph.
type edgeIterator struct {
	rows        *sql.Rows
	lastErr     error
	latchedEdge *graph.Edge
}

// Next implements graph.EdgeIterator.
func (i *edgeIterator) Next() bool {
	if i.lastErr != nil || !i.rows.Next() {
		return false
	}

	e := new(graph.Edge)
	i.lastErr = i.rows.Scan(&e.ID, &e.Src, &e.Dst, &e.UpdatedAt)
	if i.lastErr != nil {
		return false
	}
	e.UpdatedAt = e.UpdatedAt.UTC()

	i.latchedEdge = e
	return true
}

// Error implements graph.EdgeIterator.
func (i *edgeIterator) Error() error {
	return i.lastErr
}

// Close implements graph.EdgeIterator.
func (i *edgeIterator) Close() error {
	err := i.rows.Close()
	if err != nil {
		return fmt.Errorf("edge iterator: %w", err)
	}
	return nil
}

// Edge implements graph.EdgeIterator.
func (i *edgeIterator) Edge() *graph.Edge {
	return i.latchedEdge
}
