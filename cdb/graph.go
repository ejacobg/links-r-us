package cdb

import (
	"database/sql"
	"fmt"
	"github.com/ejacobg/links-r-us/graph"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"time"
)

var (
	upsertLinkQuery = `
INSERT INTO links (url, retrieved_at) VALUES ($1, $2) 
ON CONFLICT (url) DO UPDATE SET retrieved_at=GREATEST(links.retrieved_at, $2)
RETURNING id, retrieved_at
`
	findLinkQuery         = "SELECT url, retrieved_at FROM links WHERE id=$1"
	linksInPartitionQuery = "SELECT id, url, retrieved_at FROM links WHERE id >= $1 AND id < $2 AND retrieved_at < $3"

	upsertEdgeQuery = `
INSERT INTO edges (src, dst, updated_at) VALUES ($1, $2, NOW())
ON CONFLICT (src,dst) DO UPDATE SET updated_at=NOW()
RETURNING id, updated_at
`
	edgesInPartitionQuery = "SELECT id, src, dst, updated_at FROM edges WHERE src >= $1 AND src < $2 AND updated_at < $3"
	removeStaleEdgesQuery = "DELETE FROM edges WHERE src=$1 AND updated_at < $2"

	// Compile-time check for ensuring Graph implements graph.Graph.
	_ graph.Graph = (*Graph)(nil)
)

// Graph implements a graph that persists its links and edges to a
// cockroachdb instance.
type Graph struct {
	db *sql.DB
}

// NewGraph returns a Graph instance that connects to the cockroachdb
// instance specified by dsn.
func NewGraph(dsn string) (*Graph, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	return &Graph{db: db}, nil
}

// Close terminates the connection to the backing cockroachdb instance.
func (c *Graph) Close() error {
	return c.db.Close()
}

// UpsertLink creates a new link or updates an existing link.
func (c *Graph) UpsertLink(link *graph.Link) error {
	row := c.db.QueryRow(upsertLinkQuery, link.URL, link.RetrievedAt.UTC())
	if err := row.Scan(&link.ID, &link.RetrievedAt); err != nil {
		return fmt.Errorf("upsert link: %w", err)
	}

	link.RetrievedAt = link.RetrievedAt.UTC()
	return nil
}

// FindLink looks up a link by its ID.
func (c *Graph) FindLink(id uuid.UUID) (*graph.Link, error) {
	row := c.db.QueryRow(findLinkQuery, id)
	link := &graph.Link{ID: id}
	if err := row.Scan(&link.URL, &link.RetrievedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("find link: %w", graph.ErrNotFound)
		}

		return nil, fmt.Errorf("find link: %w", err)
	}

	link.RetrievedAt = link.RetrievedAt.UTC()
	return link, nil
}

// Links returns an iterator for the set of links whose IDs belong to the
// [fromID, toID) range and were last accessed before the provided value.
func (c *Graph) Links(fromID, toID uuid.UUID, accessedBefore time.Time) (graph.LinkIterator, error) {
	rows, err := c.db.Query(linksInPartitionQuery, fromID, toID, accessedBefore.UTC())
	if err != nil {
		return nil, fmt.Errorf("links: %w", err)
	}

	return &linkIterator{rows: rows}, nil
}

// UpsertEdge creates a new edge or updates an existing edge.
func (c *Graph) UpsertEdge(edge *graph.Edge) error {
	row := c.db.QueryRow(upsertEdgeQuery, edge.Src, edge.Dst)
	if err := row.Scan(&edge.ID, &edge.UpdatedAt); err != nil {
		if isForeignKeyViolationError(err) {
			err = graph.ErrUnknownEdgeLinks
		}
		return fmt.Errorf("upsert edge: %w", err)
	}

	edge.UpdatedAt = edge.UpdatedAt.UTC()
	return nil
}

// Edges returns an iterator for the set of edges whose source vertex IDs
// belong to the [fromID, toID) range and were last updated before the provided
// value.
func (c *Graph) Edges(fromID, toID uuid.UUID, updatedBefore time.Time) (graph.EdgeIterator, error) {
	rows, err := c.db.Query(edgesInPartitionQuery, fromID, toID, updatedBefore.UTC())
	if err != nil {
		return nil, fmt.Errorf("edges: %w", err)
	}

	return &edgeIterator{rows: rows}, nil
}

// RemoveStaleEdges removes any edge that originates from the specified link ID
// and was updated before the specified timestamp.
func (c *Graph) RemoveStaleEdges(fromID uuid.UUID, updatedBefore time.Time) error {
	_, err := c.db.Exec(removeStaleEdgesQuery, fromID, updatedBefore.UTC())
	if err != nil {
		return fmt.Errorf("remove stale edges: %w", err)
	}

	return nil
}

// isForeignKeyViolationError returns true if err indicates a foreign key
// constraint violation.
func isForeignKeyViolationError(err error) bool {
	pqErr, valid := err.(*pq.Error)
	if !valid {
		return false
	}

	return pqErr.Code.Name() == "foreign_key_violation"
}
