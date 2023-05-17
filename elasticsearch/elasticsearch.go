package elasticsearch

import (
	"fmt"
	"time"
)

// The name of the elasticsearch index to use.
const indexName = "textindexer"

// The size of each page of results that is cached locally by the iterator.
const batchSize = 10

var mappings = `
{
  "mappings" : {
    "properties": {
      "LinkID": {"type": "keyword"},
      "URL": {"type": "keyword"},
      "Content": {"type": "text"},
      "Title": {"type": "text"},
      "IndexedAt": {"type": "date"},
      "PageRank": {"type": "double"}
    }
  }
}`

type searchResult struct {
	Hits searchResultHits `json:"hits"`
}

type searchResultHits struct {
	Total   total        `json:"total"`
	HitList []hitWrapper `json:"hits"`
}

type total struct {
	Count uint64 `json:"value"`
}

type hitWrapper struct {
	DocSource document `json:"_source"`
}

type document struct {
	LinkID    string    `json:"LinkID"`
	URL       string    `json:"URL"`
	Title     string    `json:"Title"`
	Content   string    `json:"Content"`
	IndexedAt time.Time `json:"IndexedAt"`
	PageRank  float64   `json:"PageRank,omitempty"`
}

type updateResult struct {
	Result string `json:"result"`
}

// Not all errors can be marshaled into this struct, so unmarshalError may fail.
// You should probably decode into a map[string]any as shown in the docs: https://github.com/elastic/go-elasticsearch#usage
type errorResult struct {
	Error esError `json:"error"`
}

type esError struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

func (e esError) Error() string {
	return fmt.Sprintf("%s: %s", e.Type, e.Reason)
}
