package crawler

import (
	"context"
	"github.com/ejacobg/links-r-us/graph"
	"github.com/ejacobg/links-r-us/pipeline"
)

// Config encapsulates the configuration options for creating a new Crawler.
type Config struct {
	// A PrivateNetworkDetector instance
	PrivateNetworkDetector PrivateNetworkDetector

	// A URLGetter instance for fetching links.
	URLGetter URLGetter

	// A GraphUpdater instance for adding new links to the link graph.
	Graph Graph

	// A TextIndexer instance for indexing the content of each retrieved link.
	Indexer Indexer

	// The number of concurrent workers used for retrieving links.
	FetchWorkers int
}

// Crawler implements a web-page crawling pipeline consisting of the following
// stages:
//
//   - Given a URL, retrieve the web-page contents from the remote server.
//   - Extract and resolve absolute and relative links from the retrieved page.
//   - Extract page title and text content from the retrieved page.
//   - Update the link graph: add new links and create edges between the crawled
//     page and the links within it.
//   - Index crawled page title and text content.
type Crawler struct {
	p *pipeline.Pipeline
}

// NewCrawler returns a new crawler instance.
func NewCrawler(cfg Config) *Crawler {
	return &Crawler{
		p: assembleCrawlerPipeline(cfg),
	}
}

// assembleCrawlerPipeline creates the various stages of a crawler pipeline
// using the options in cfg and assembles them into a pipeline instance.
func assembleCrawlerPipeline(cfg Config) *pipeline.Pipeline {
	return pipeline.New(
		pipeline.FixedWorkerPool(
			newLinkFetcher(cfg.URLGetter, cfg.PrivateNetworkDetector),
			cfg.FetchWorkers,
		),
		pipeline.FIFO(newLinkExtractor(cfg.PrivateNetworkDetector)),
		pipeline.FIFO(newTextExtractor()),
		pipeline.Broadcast(
			newGraphUpdater(cfg.Graph),
			newTextIndexer(cfg.Indexer),
		),
	)
}

// Crawl iterates linkIt and sends each link through the crawler pipeline
// returning the total count of links that went through the pipeline. Calls to
// Crawl block until the link iterator is exhausted, an error occurs or the
// context is cancelled.
func (c *Crawler) Crawl(ctx context.Context, linkIt graph.LinkIterator) (int, error) {
	sink := new(countingSink)
	err := c.p.Process(ctx, &linkSource{linkIt: linkIt}, sink)
	return sink.getCount(), err
}

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

type countingSink struct {
	count int
}

func (s *countingSink) Consume(_ context.Context, p pipeline.Payload) error {
	s.count++
	return nil
}

func (s *countingSink) getCount() int {
	// The broadcast split-stage sends out two payloads for each incoming link
	// so we need to divide the total count by 2.
	return s.count / 2
}

type nopSink struct{}

func (nopSink) Consume(context.Context, pipeline.Payload) error {
	return nil
}
