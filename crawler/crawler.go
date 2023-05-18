package crawler

import (
	"context"
	"github.com/ejacobg/links-r-us/graph"
	"github.com/ejacobg/links-r-us/pipeline"
)

type linkSource struct {
	linkIt graph.LinkIterator
}

func (ls *linkSource) Error() error              { return ls.linkIt.Error() }
func (ls *linkSource) Next(context.Context) bool { return ls.linkIt.Next() }
func (ls *linkSource) Payload() pipeline.Payload {
	link := ls.linkIt.Link()
	p := payloadPool.Get().(*crawlerPayload)

	p.LinkID = link.ID
	p.URL = link.URL
	p.RetrievedAt = link.RetrievedAt
	return p
}

type nopSink struct{}

func (nopSink) Consume(context.Context, pipeline.Payload) error {
	return nil
}
