package bleve

import (
	"github.com/blevesearch/bleve/v2"
	"github.com/ejacobg/links-r-us/index"
)

// iterator implements index.Iterator.
type iterator struct {
	// idx stores a reference to our index, which we use to access the stored documents.
	idx *Indexer

	// searchReq represents the original request. This is needed in case more results are needed.
	searchReq *bleve.SearchRequest

	cumIdx uint64 // Tracks the position relative to all items in the result list.
	rsIdx  int    // Tracks the position relative to the current page of results.
	rs     *bleve.SearchResult

	latchedDoc *index.Document
	lastErr    error
}

// Close the iterator and release any allocated resources.
func (it *iterator) Close() error {
	it.idx = nil
	it.searchReq = nil
	if it.rs != nil {
		it.cumIdx = it.rs.Total
	}
	return nil
}

// Next loads the next document matching the search query.
// It returns false if no more documents are available.
func (it *iterator) Next() bool {
	if it.lastErr != nil || it.rs == nil || it.cumIdx >= it.rs.Total {
		return false
	}

	// Do we need to fetch the next batch?
	if it.rsIdx >= it.rs.Hits.Len() {
		// Request the next page of results.
		it.searchReq.From += it.searchReq.Size
		if it.rs, it.lastErr = it.idx.idx.Search(it.searchReq); it.lastErr != nil {
			return false
		}

		// Set our progress to the top of the page.
		it.rsIdx = 0
	}

	// Obtain a reference to the next document. Remember that findByID() will return a copy.
	nextID := it.rs.Hits[it.rsIdx].ID
	if it.latchedDoc, it.lastErr = it.idx.findByID(nextID); it.lastErr != nil {
		return false
	}

	it.cumIdx++
	it.rsIdx++
	return true
}

// Error returns the last error encountered by the iterator.
func (it *iterator) Error() error {
	return it.lastErr
}

// Document returns the current document from the result set.
func (it *iterator) Document() *index.Document {
	return it.latchedDoc
}

// TotalCount returns the approximate number of search results.
func (it *iterator) TotalCount() uint64 {
	if it.rs == nil {
		return 0
	}
	return it.rs.Total
}
