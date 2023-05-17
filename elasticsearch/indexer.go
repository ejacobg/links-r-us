package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ejacobg/links-r-us/index"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/google/uuid"
	"strings"
)

// Compile-time check to ensure Indexer implements index.Indexer.
var _ index.Indexer = (*Indexer)(nil)

// Indexer is an index.Indexer implementation that uses an elastic search
// instance to catalogue and search documents.
type Indexer struct {
	es         *elasticsearch.Client
	refreshOpt func(*esapi.UpdateRequest)
}

// NewIndexer creates a text indexer that uses an elastic search instance
// for indexing documents.
func NewIndexer(nodes []string, syncUpdates bool) (*Indexer, error) {
	cfg := elasticsearch.Config{
		Addresses: nodes,
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	if err = ensureIndex(es); err != nil {
		return nil, err
	}

	refreshOpt := es.Update.WithRefresh("false")
	if syncUpdates {
		refreshOpt = es.Update.WithRefresh("true")
	}

	return &Indexer{
		es:         es,
		refreshOpt: refreshOpt,
	}, nil
}

// Index inserts a new document to the index or updates the index entry
// for and existing document.
func (i *Indexer) Index(d *index.Document) error {
	if d.LinkID == uuid.Nil {
		return fmt.Errorf("index: %w", index.ErrMissingLinkID)
	}

	var (
		buf bytes.Buffer
		doc = makeDoc(d) // Make a copy of the document that's usable by Elasticsearch.
	)

	// Create our update request.
	update := map[string]interface{}{
		"doc":           doc,
		"doc_as_upsert": true,
	}

	// Because we did not set a PageRank value, it will not be marshaled into the request because of the `omitempty` tag.
	if err := json.NewEncoder(&buf).Encode(update); err != nil {
		return fmt.Errorf("index: %w", err)
	}

	// Send our request to our cluster.
	res, err := i.es.Update(indexName, doc.LinkID, &buf, i.refreshOpt)

	// nil errors typically signify things like a failed DNS lookup or a failed connection.
	if err != nil {
		return fmt.Errorf("index: %w", err)
	}

	// A structured JSON response with error details may also be returned.
	// If an error is present, we will return it, otherwise the result will be marshaled into the updateRes variable.
	var updateRes updateResult
	if err = unmarshalResponse(res, &updateRes); err != nil {
		return fmt.Errorf("index: %w", err)
	}

	// In this case, we don't do anything with the update result.

	return nil
}

// FindByID looks up a document by its link ID.
func (i *Indexer) FindByID(linkID uuid.UUID) (*index.Document, error) {
	var buf bytes.Buffer
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"match": map[string]interface{}{
				"LinkID": linkID.String(),
			},
		},
		"from": 0,
		"size": 1,
	}
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return nil, fmt.Errorf("find by ID: %w", err)
	}

	searchRes, err := runSearch(i.es, query)
	if err != nil {
		return nil, fmt.Errorf("find by ID: %w", err)
	}

	if len(searchRes.Hits.HitList) != 1 {
		return nil, fmt.Errorf("find by ID: %w", index.ErrNotFound)
	}

	return mapDoc(&searchRes.Hits.HitList[0].DocSource), nil
}

// Search the index for a particular query and return back a result
// iterator.
func (i *Indexer) Search(q index.Query) (index.Iterator, error) {
	var qtype string
	switch q.Type {
	case index.QueryTypePhrase:
		qtype = "phrase"
	default:
		qtype = "best_fields"
	}

	query := map[string]interface{}{
		"query": map[string]interface{}{
			"function_score": map[string]interface{}{
				"query": map[string]interface{}{
					"multi_match": map[string]interface{}{
						"type":   qtype,
						"query":  q.Expression,
						"fields": []string{"Title", "Content"},
					},
				},
				"script_score": map[string]interface{}{
					"script": map[string]interface{}{
						// Augment Elasticsearch's calculated relevance score with each document's PageRank value.
						"source": "_score + doc['PageRank'].value",
					},
				},
			},
		},
		// Fields for handling the page offset and page size.
		"from": q.Offset,
		"size": batchSize,
	}

	searchRes, err := runSearch(i.es, query)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	return &iterator{es: i.es, searchReq: query, rs: searchRes, cumIdx: q.Offset}, nil
}

// UpdateScore updates the PageRank score for a document with the
// specified link ID. If no such document exists, a placeholder
// document with the provided score will be created.
func (i *Indexer) UpdateScore(linkID uuid.UUID, score float64) error {
	var buf bytes.Buffer
	update := map[string]interface{}{
		"doc": map[string]interface{}{
			"LinkID":   linkID.String(),
			"PageRank": score,
		},
		"doc_as_upsert": true,
	}
	if err := json.NewEncoder(&buf).Encode(update); err != nil {
		return fmt.Errorf("update score: %w", err)
	}

	res, err := i.es.Update(indexName, linkID.String(), &buf, i.refreshOpt)
	if err != nil {
		return fmt.Errorf("update score: %w", err)
	}

	var updateRes updateResult
	if err = unmarshalResponse(res, &updateRes); err != nil {
		return fmt.Errorf("update score: %w", err)
	}

	return nil
}

// ensureIndex creates a new index with the predefined mappings on the given client.
func ensureIndex(es *elasticsearch.Client) error {
	mappingsReader := strings.NewReader(mappings)
	res, err := es.Indices.Create(indexName, es.Indices.Create.WithBody(mappingsReader))
	if err != nil {
		return fmt.Errorf("cannot create ES index: %w", err)
	} else if res.IsError() {
		err := unmarshalError(res)
		if esErr, valid := err.(esError); valid && esErr.Type == "resource_already_exists_exception" {
			return nil
		}
		return fmt.Errorf("cannot create ES index: %w", err)
	}

	return nil
}

// runSearch submits the given search query to the Elasticsearch cluster.
func runSearch(es *elasticsearch.Client, searchQuery map[string]interface{}) (*searchResult, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(searchQuery); err != nil {
		return nil, fmt.Errorf("find by ID: %w", err)
	}

	// Perform the search request.
	res, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex(indexName),
		es.Search.WithBody(&buf),
	)
	if err != nil {
		return nil, err
	}

	var searchRes searchResult
	if err = unmarshalResponse(res, &searchRes); err != nil {
		return nil, err
	}

	return &searchRes, nil
}

func unmarshalError(res *esapi.Response) error {
	return unmarshalResponse(res, nil)
}

func unmarshalResponse(res *esapi.Response, to interface{}) error {
	defer func() { _ = res.Body.Close() }()

	if res.IsError() {
		var errRes errorResult
		fmt.Println(res.String())
		if err := json.NewDecoder(res.Body).Decode(&errRes); err != nil {
			return err
		}

		return errRes.Error
	}

	return json.NewDecoder(res.Body).Decode(to)
}

// mapDoc converts a document into an index.Document.
func mapDoc(d *document) *index.Document {
	return &index.Document{
		LinkID:    uuid.MustParse(d.LinkID),
		URL:       d.URL,
		Title:     d.Title,
		Content:   d.Content,
		IndexedAt: d.IndexedAt.UTC(),
		PageRank:  d.PageRank,
	}
}

func makeDoc(d *index.Document) document {
	// Note: we intentionally skip PageRank as we don't want updates to
	// overwrite existing PageRank values.
	return document{
		LinkID:    d.LinkID.String(),
		URL:       d.URL,
		Title:     d.Title,
		Content:   d.Content,
		IndexedAt: d.IndexedAt.UTC(),
	}
}
