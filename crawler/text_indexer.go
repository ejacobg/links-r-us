package crawler

import (
	"context"
	"github.com/ejacobg/links-r-us/index"
	"github.com/ejacobg/links-r-us/pipeline"
	"time"
)

// Indexer is implemented by objects that can index the contents of web-pages
// retrieved by the crawler pipeline.
type Indexer interface {
	// Index inserts a new document to the index or updates the index entry
	// for and existing document.
	Index(doc *index.Document) error
}

type textIndexer struct {
	indexer Indexer
}

func newTextIndexer(indexer Indexer) *textIndexer {
	return &textIndexer{
		indexer: indexer,
	}
}

func (i *textIndexer) Process(ctx context.Context, p pipeline.Payload) (pipeline.Payload, error) {
	payload := p.(*crawlerPayload)

	doc := &index.Document{
		LinkID:    payload.LinkID,
		URL:       payload.URL,
		Title:     payload.Title,
		Content:   payload.TextContent,
		IndexedAt: time.Now(),
	}
	if err := i.indexer.Index(doc); err != nil {
		return nil, err
	}

	return p, nil
}
