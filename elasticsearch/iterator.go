package elasticsearch

import (
	"github.com/ejacobg/links-r-us/index"
	"github.com/elastic/go-elasticsearch/v8"
)

// The implementation details here are very similar to that of bleve.iterator.

// iterator implements index.Iterator.
type iterator struct {
	es        *elasticsearch.Client
	searchReq map[string]interface{}

	cumIdx uint64
	rsIdx  int
	rs     *searchResult

	latchedDoc *index.Document
	lastErr    error
}

// Close the iterator and release any allocated resources.
func (it *iterator) Close() error {
	it.es = nil
	it.searchReq = nil
	it.cumIdx = it.rs.Hits.Total.Count
	return nil
}

// Next loads the next document matching the search query.
// It returns false if no more documents are available.
func (it *iterator) Next() bool {
	if it.lastErr != nil || it.rs == nil || it.cumIdx >= it.rs.Hits.Total.Count {
		return false
	}

	// Do we need to fetch the next batch?
	if it.rsIdx >= len(it.rs.Hits.HitList) {
		it.searchReq["from"] = it.searchReq["from"].(uint64) + batchSize
		if it.rs, it.lastErr = runSearch(it.es, it.searchReq); it.lastErr != nil {
			return false
		}

		it.rsIdx = 0
	}

	it.latchedDoc = mapDoc(&it.rs.Hits.HitList[it.rsIdx].DocSource)
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
	return it.rs.Hits.Total.Count
}
