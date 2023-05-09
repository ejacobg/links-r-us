// Package inmem provides an in-memory graph implementation.
package inmem

import (
	"fmt"
	"github.com/ejacobg/links-r-us/graph"
	"github.com/google/uuid"
	"sync"
	"time"
)

// Compile-time check for ensuring InMemoryGraph implements Graph.
var _ graph.Graph = (*InMemoryGraph)(nil)

// edgeList represents all of a link's outgoing edges. In other words, all the edges with this link as the source.
type edgeList []uuid.UUID

// InMemoryGraph implements an in-memory link graph that can be concurrently
// accessed by multiple clients.
type InMemoryGraph struct {
	// Unlike sync.Mutex, sync.RWMutex supports multiple-reader semantics, good for read-heavy workloads.
	mu sync.RWMutex

	links map[uuid.UUID]*graph.Link
	edges map[uuid.UUID]*graph.Edge

	// Link URLs are expected to be unique. Use this to check for uniqueness.
	linkFromURL map[string]*graph.Link

	// Used to easily obtain a link's outgoing edges.
	linkEdges map[uuid.UUID]edgeList
}

// NewInMemoryGraph creates a new in-memory link graph.
func NewInMemoryGraph() *InMemoryGraph {
	return &InMemoryGraph{
		links:       make(map[uuid.UUID]*graph.Link),
		edges:       make(map[uuid.UUID]*graph.Edge),
		linkFromURL: make(map[string]*graph.Link),
		linkEdges:   make(map[uuid.UUID]edgeList),
	}
}

// UpsertLink creates a new link or updates an existing link.
func (im *InMemoryGraph) UpsertLink(link *graph.Link) error {
	// Writes will always update the graph. Obtain a writer lock.
	im.mu.Lock()
	defer im.mu.Unlock()

	// Check if a link with the same URL already exists. If so, convert
	// this into an update and point the link ID to the existing link.
	if existing := im.linkFromURL[link.URL]; existing != nil {
		link.ID = existing.ID

		// Copy data into the saved version, but keep the most recent timestamp.
		origTs := existing.RetrievedAt
		*existing = *link
		if origTs.After(existing.RetrievedAt) {
			existing.RetrievedAt = origTs
		}

		return nil
	}

	// Assign new ID and insert link.
	for {
		link.ID = uuid.New()
		if im.links[link.ID] == nil {
			break
		}
	}

	// Add a copy of the new link to the graph.
	lCopy := new(graph.Link)
	*lCopy = *link
	im.linkFromURL[lCopy.URL] = lCopy
	im.links[lCopy.ID] = lCopy
	return nil
}

// FindLink looks up a copy of a link by its ID.
func (im *InMemoryGraph) FindLink(id uuid.UUID) (*graph.Link, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	link := im.links[id]
	if link == nil {
		return nil, fmt.Errorf("find link: %w", graph.ErrNotFound)
	}

	lCopy := new(graph.Link)
	*lCopy = *link
	return lCopy, nil
}

// Links returns an iterator for the set of links whose IDs belong to the
// [fromID, toID) range and were retrieved before the provided timestamp.
func (im *InMemoryGraph) Links(fromID, toID uuid.UUID, retrievedBefore time.Time) (graph.LinkIterator, error) {
	// UUID values can be compared directly (which is faster), however we are converting to strings for debugging purposes.
	from, to := fromID.String(), toID.String()

	im.mu.RLock()
	var list []*graph.Link
	for linkID, link := range im.links {
		if id := linkID.String(); id >= from && id < to && link.RetrievedAt.Before(retrievedBefore) {
			list = append(list, link)
		}
	}
	im.mu.RUnlock()

	return &linkIterator{im: im, links: list}, nil
}

// UpsertEdge creates a new edge or updates an existing edge.
func (im *InMemoryGraph) UpsertEdge(edge *graph.Edge) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	_, srcExists := im.links[edge.Src]
	_, dstExists := im.links[edge.Dst]
	if !srcExists || !dstExists {
		return fmt.Errorf("upsert edge: %w", graph.ErrUnknownEdgeLinks)
	}

	// Scan the source's edge list to see if this edge has been recorded before.
	for _, edgeID := range im.linkEdges[edge.Src] {
		existing := im.edges[edgeID]
		// Technically we don't need to check if the sources are the same since all edges in the list have the same source.
		if existing.Src == edge.Src && existing.Dst == edge.Dst {
			// Update the timestamp and copy our saved data into this edge.
			existing.UpdatedAt = time.Now()
			*edge = *existing
			return nil
		}
	}

	// Assign new ID and insert edge.
	for {
		edge.ID = uuid.New()
		if im.edges[edge.ID] == nil {
			break
		}
	}

	// Add a copy of the new edge to the graph.
	edge.UpdatedAt = time.Now()
	eCopy := new(graph.Edge)
	*eCopy = *edge
	im.edges[eCopy.ID] = eCopy

	// Append the edge ID to the list of edges originating from the
	// edge's source link.
	im.linkEdges[edge.Src] = append(im.linkEdges[edge.Src], eCopy.ID)
	return nil
}

// Edges returns an iterator for the set of edges whose source vertex IDs
// belong to the [fromID, toID) range and were updated before the provided
// timestamp.
func (im *InMemoryGraph) Edges(fromID, toID uuid.UUID, updatedBefore time.Time) (graph.EdgeIterator, error) {
	from, to := fromID.String(), toID.String()

	im.mu.RLock()
	var list []*graph.Edge
	for linkID := range im.links {
		// If a link does not fall within our range, then we can ignore all the edges originating from it.
		if id := linkID.String(); id < from || id >= to {
			continue
		}

		for _, edgeID := range im.linkEdges[linkID] {
			if edge := im.edges[edgeID]; edge.UpdatedAt.Before(updatedBefore) {
				list = append(list, edge)
			}
		}
	}
	im.mu.RUnlock()

	return &edgeIterator{im: im, edges: list}, nil
}

// RemoveStaleEdges removes any edge that originates from the specified link ID
// and was updated before the specified timestamp.
func (im *InMemoryGraph) RemoveStaleEdges(fromID uuid.UUID, updatedBefore time.Time) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	var freshEdges edgeList
	for _, edgeID := range im.linkEdges[fromID] {
		edge := im.edges[edgeID]
		if edge.UpdatedAt.Before(updatedBefore) {
			delete(im.edges, edgeID)
			continue
		}

		freshEdges = append(freshEdges, edgeID)
	}

	// Replace edge list or origin link with the filtered edge list
	im.linkEdges[fromID] = freshEdges
	return nil
}
