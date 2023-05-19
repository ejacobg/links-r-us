package crawler

import (
	"context"
	"github.com/ejacobg/links-r-us/graph"
	"github.com/ejacobg/links-r-us/pipeline"
	"github.com/google/uuid"
	"time"
)

// Graph is implemented by objects that can upsert links and edges into a link
// graph instance.
type Graph interface {
	// UpsertLink creates a new link or updates an existing link.
	UpsertLink(link *graph.Link) error

	// UpsertEdge creates a new edge or updates an existing edge.
	UpsertEdge(edge *graph.Edge) error

	// RemoveStaleEdges removes any edge that originates from the specified
	// link ID and was updated before the specified timestamp.
	RemoveStaleEdges(fromID uuid.UUID, updatedBefore time.Time) error
}

type graphUpdater struct {
	updater Graph
}

func newGraphUpdater(updater Graph) *graphUpdater {
	return &graphUpdater{
		updater: updater,
	}
}

func (u *graphUpdater) Process(ctx context.Context, p pipeline.Payload) (pipeline.Payload, error) {
	payload := p.(*crawlerPayload)

	// Extract the link from the payload.
	src := &graph.Link{
		ID:          payload.LinkID,
		URL:         payload.URL,
		RetrievedAt: time.Now(),
	}

	// If the link isn't present in the graph, it will be added, otherwise the timestamp will be updated.
	if err := u.updater.UpsertLink(src); err != nil {
		return nil, err
	}

	// Add the nofollow links to the graph, but do not create edges to them.
	for _, dstLink := range payload.NoFollowLinks {
		dst := &graph.Link{URL: dstLink}
		if err := u.updater.UpsertLink(dst); err != nil {
			return nil, err
		}
	}

	// Upsert discovered links and create edges for them. Keep track of
	// the current time so we can drop stale edges that have not been
	// updated after this loop.
	removeEdgesOlderThan := time.Now()
	for _, dstLink := range payload.Links {
		dst := &graph.Link{URL: dstLink}

		if err := u.updater.UpsertLink(dst); err != nil {
			return nil, err
		}

		if err := u.updater.UpsertEdge(&graph.Edge{Src: src.ID, Dst: dst.ID}); err != nil {
			return nil, err
		}
	}

	// Drop stale edges that were not touched while upserting the outgoing
	// edges.
	if err := u.updater.RemoveStaleEdges(src.ID, removeEdgesOlderThan); err != nil {
		return nil, err
	}

	return p, nil
}
