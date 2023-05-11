package bleve

import (
	"fmt"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/ejacobg/links-r-us/index"
	"github.com/google/uuid"
	"sync"
	"time"
)

// The size of each page of results that is cached locally by the iterator.
const batchSize = 10

// Compile-time check to ensure Indexer implements index.Indexer.
var _ index.Indexer = (*Indexer)(nil)

// document is a subset of index.Document.
type document struct {
	Title    string
	Content  string
	PageRank float64
}

// Indexer is an index.Indexer implementation that uses an in-memory
// bleve instance to catalogue and search documents.
type Indexer struct {
	mu sync.RWMutex

	// docs maps a link ID to the document it represents.
	// Documents in this map are considered immutable.
	docs map[string]*index.Document

	// idx represents the bleve index on which to perform queries.
	idx bleve.Index
}

// NewIndexer creates a text indexer that uses an in-memory
// bleve instance for indexing documents.
func NewIndexer() (*Indexer, error) {
	mapping := bleve.NewIndexMapping()
	idx, err := bleve.NewMemOnly(mapping)
	if err != nil {
		return nil, err
	}

	return &Indexer{
		idx:  idx,
		docs: make(map[string]*index.Document),
	}, nil
}

// Close the indexer and release any allocated resources.
func (i *Indexer) Close() error {
	return i.idx.Close()
}

// Index inserts a new document to the index or updates the index entry
// for and existing document.
func (i *Indexer) Index(doc *index.Document) error {
	if doc.LinkID == uuid.Nil {
		return fmt.Errorf("index: %w", index.ErrMissingLinkID)
	}

	doc.IndexedAt = time.Now()

	// A copy of the document will be made which will later be saved into our internal document map.
	// A copy needs to be made so that the user does not hold a reference to it.
	dcopy := copyDoc(doc)

	// The bleve index uses string based keys.
	key := dcopy.LinkID.String()

	i.mu.Lock()
	defer i.mu.Unlock()

	// The index operation should not update the PageRank score.
	// Overwrite any PageRank score applied by the copy operation with the original score.
	if orig, exists := i.docs[key]; exists {
		dcopy.PageRank = orig.PageRank
	}

	// Use makeDoc() to store a partial document into the index.
	if err := i.idx.Index(key, makeDoc(dcopy)); err != nil {
		return fmt.Errorf("index: %w", err)
	}

	i.docs[key] = dcopy
	return nil
}

// FindByID looks up a document by its link ID.
func (i *Indexer) FindByID(linkID uuid.UUID) (*index.Document, error) {
	return i.findByID(linkID.String())
}

// findByID looks up a document by its link UUID expressed as a string.
func (i *Indexer) findByID(linkID string) (*index.Document, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if d, found := i.docs[linkID]; found {
		return copyDoc(d), nil
	}

	return nil, fmt.Errorf("find by ID: %w", index.ErrNotFound)
}

// Search the index for a particular query and return back a result
// iterator.
func (i *Indexer) Search(q index.Query) (index.Iterator, error) {
	// Create the appropriate query from the given expression.
	var bq query.Query
	switch q.Type {
	case index.QueryTypePhrase:
		bq = bleve.NewMatchPhraseQuery(q.Expression)
	default:
		bq = bleve.NewMatchQuery(q.Expression)
	}

	searchReq := bleve.NewSearchRequest(bq)
	searchReq.SortBy([]string{"-PageRank", "-_score"}) // Order by PageRank score, in descending order.
	searchReq.Size = batchSize                         // Bleve results are always paginated. Use this to control the page size.
	searchReq.From = int(q.Offset)                     // Controls the page offset.
	rs, err := i.idx.Search(searchReq)                 // Submit the search request.
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	return &iterator{idx: i, searchReq: searchReq, rs: rs, cumIdx: q.Offset}, nil
}

// UpdateScore updates the PageRank score for a document with the specified
// link ID. If no such document exists, a placeholder document with the
// provided score will be created.
func (i *Indexer) UpdateScore(linkID uuid.UUID, score float64) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	key := linkID.String()
	doc, found := i.docs[key]
	if !found {
		doc = &index.Document{LinkID: linkID}
		i.docs[key] = doc
	}

	doc.PageRank = score
	if err := i.idx.Index(key, makeDoc(doc)); err != nil {
		return fmt.Errorf("update score: %w", err)
	}

	return nil
}

func copyDoc(d *index.Document) *index.Document {
	dcopy := new(index.Document)
	*dcopy = *d
	return dcopy
}

func makeDoc(d *index.Document) document {
	return document{
		Title:    d.Title,
		Content:  d.Content,
		PageRank: d.PageRank,
	}
}
