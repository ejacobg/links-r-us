package indextest

import (
	"errors"
	"fmt"
	"github.com/ejacobg/links-r-us/index"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"testing"
	"time"
)

// Sidenote: should probably use `return` instead of t.Fatalf() so that the AfterEach() function gets a chance to execute.

// Suite defines a re-usable set of index-related tests that can
// be executed against any type that implements index.Indexer.
type Suite struct {
	Idx index.Indexer

	// Optional helper functions.
	BeforeEach func(t *testing.T)
	AfterEach  func(t *testing.T)
}

// TestIndexer runs all the below functions on the index.
func (s *Suite) TestIndexer(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*testing.T, index.Indexer)
	}{
		{"Index document", TestIndexDocument},
		{"Index does not override pagerank", TestIndexDoesNotOverridePageRank},
		{"Find by ID", TestFindByID},
		{"Phrase search", TestPhraseSearch},
		{"Match search", TestMatchSearch},
		{"Match search with offset", TestMatchSearchWithOffset},
		{"Update score", TestUpdateScore},
		{"Update score for unknown document", TestUpdateScoreForUnknownDocument},
	}

	if s.BeforeEach == nil {
		s.BeforeEach = func(t *testing.T) {}
	}

	if s.AfterEach == nil {
		s.AfterEach = func(t *testing.T) {}
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s.BeforeEach(t)
			test.fn(t, s.Idx)
			s.AfterEach(t)
		})
	}
}

// TestIndexDocument verifies the indexing logic for new and existing documents.
func TestIndexDocument(t *testing.T, idx index.Indexer) {
	// Insert a document without an ID.
	incompleteDoc := &index.Document{
		URL: "http://example.com",
	}

	if err := idx.Index(incompleteDoc); !errors.Is(err, index.ErrMissingLinkID) {
		t.Errorf("unexpected error %v, want %v", err, index.ErrMissingLinkID)
	}

	// Insert a new document.
	doc := &index.Document{
		LinkID:    uuid.New(),
		URL:       "http://example.com",
		Title:     "Illustrious examples",
		Content:   "Lorem ipsum dolor",
		IndexedAt: time.Now().Add(-12 * time.Hour).UTC(),
	}

	if err := idx.Index(doc); err != nil {
		t.Fatalf("could not index document: %v", err)
	}

	// Update existing document.
	doc.URL = "http://example.com"
	doc.Title = "A more exciting title"
	doc.Content = "Ovidius poeta in terra pontica"
	doc.IndexedAt = time.Now().UTC()

	if err := idx.Index(doc); err != nil {
		t.Errorf("could not update document: %v", err)
	}
}

func TestIndexDoesNotOverridePageRank(t *testing.T, idx index.Indexer) {
	// Insert a new document.
	doc := &index.Document{
		LinkID:    uuid.New(),
		URL:       "http://example.com",
		Title:     "Illustrious examples",
		Content:   "Lorem ipsum dolor",
		IndexedAt: time.Now().Add(-12 * time.Hour).UTC(),
	}

	if err := idx.Index(doc); err != nil {
		t.Fatalf("could not index document: %v", err)
	}

	// Update the score.
	score := 0.5
	if err := idx.UpdateScore(doc.LinkID, score); err != nil {
		t.Fatalf("could not update score: %v", err)
	}

	// Update existing document.
	doc.URL = "http://example.com"
	doc.Title = "A more exciting title"
	doc.Content = "Ovidius poeta in terra pontica"
	doc.IndexedAt = time.Now().UTC()

	if err := idx.Index(doc); err != nil {
		t.Errorf("could not update document: %v", err)
	}

	// Look up document and verify that PageRank score has not been changed.
	got, err := idx.FindByID(doc.LinkID)
	if err != nil {
		t.Fatalf("could not find document: %v", err)
	}

	if got.PageRank != score {
		t.Errorf("pagerank score = %f, want %f", got.PageRank, score)
	}
}

// TestFindByID verifies the document lookup logic.
func TestFindByID(t *testing.T, idx index.Indexer) {
	// Insert a new document.
	doc := &index.Document{
		LinkID:    uuid.New(),
		URL:       "http://example.com",
		Title:     "Illustrious examples",
		Content:   "Lorem ipsum dolor",
		IndexedAt: time.Now().Add(-12 * time.Hour).UTC(),
	}

	if err := idx.Index(doc); err != nil {
		t.Fatalf("could not index document: %v", err)
	}

	// Look up document and confirm all fields match.
	got, err := idx.FindByID(doc.LinkID)
	if err != nil {
		t.Fatalf("could not find document: %v", err)
	}

	if !cmp.Equal(got, doc) {
		t.Errorf("document returned by FindByID does not match inserted document")
	}

	// Look up unknown ID.
	_, err = idx.FindByID(uuid.New())
	if !errors.Is(err, index.ErrNotFound) {
		t.Errorf("unexpected error %v, want %v", err, index.ErrNotFound)
	}
}

// TestPhraseSearch verifies the document search logic when searching for
// exact phrases.
func TestPhraseSearch(t *testing.T, idx index.Indexer) {
	var (
		numDocs = 50
		expIDs  []uuid.UUID
	)
	for i := 0; i < numDocs; i++ {
		id := uuid.New()
		doc := &index.Document{
			LinkID:  id,
			Title:   fmt.Sprintf("doc with ID %s", id.String()),
			Content: "Lorem Ipsum Dolor",
		}

		if i%5 == 0 {
			doc.Content = "Lorem Dolor Ipsum"
			expIDs = append(expIDs, id)
		}

		if err := idx.Index(doc); err != nil {
			t.Fatalf("could not index document: %v", err)
		}

		if err := idx.UpdateScore(doc.LinkID, float64(numDocs-i)); err != nil {
			t.Fatalf("could not update score: %v", err)
		}
	}

	it, err := idx.Search(index.Query{
		Type:       index.QueryTypePhrase,
		Expression: "lorem dolor ipsum",
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	ids := iterateDocs(t, it)
	if !cmp.Equal(ids, expIDs) {
		t.Errorf("search returned incorrect IDs")
	}
}

// TestMatchSearch verifies the document search logic when searching for
// keyword matches.
func TestMatchSearch(t *testing.T, idx index.Indexer) {
	var (
		numDocs = 50
		expIDs  []uuid.UUID
	)
	for i := 0; i < numDocs; i++ {
		id := uuid.New()
		doc := &index.Document{
			LinkID:  id,
			Title:   fmt.Sprintf("doc with ID %s", id.String()),
			Content: "Ovidius poeta in terra pontica",
		}

		if i%5 == 0 {
			doc.Content = "Lorem Dolor Ipsum"
			expIDs = append(expIDs, id)
		}

		if err := idx.Index(doc); err != nil {
			t.Fatalf("could not index document: %v", err)
		}

		if err := idx.UpdateScore(doc.LinkID, float64(numDocs-i)); err != nil {
			t.Fatalf("could not update score: %v", err)
		}
	}

	it, err := idx.Search(index.Query{
		Type:       index.QueryTypeMatch,
		Expression: "lorem ipsum",
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	ids := iterateDocs(t, it)
	if !cmp.Equal(ids, expIDs) {
		t.Errorf("search returned incorrect IDs")
	}
}

// TestMatchSearchWithOffset verifies the document search logic when searching
// for keyword matches and skipping some results.
func TestMatchSearchWithOffset(t *testing.T, idx index.Indexer) {
	var (
		numDocs = 50
		expIDs  []uuid.UUID
	)
	for i := 0; i < numDocs; i++ {
		id := uuid.New()
		expIDs = append(expIDs, id)
		doc := &index.Document{
			LinkID:  id,
			Title:   fmt.Sprintf("doc with ID %s", id.String()),
			Content: "Ovidius poeta in terra pontica",
		}

		if err := idx.Index(doc); err != nil {
			t.Fatalf("could not index document: %v", err)
		}

		if err := idx.UpdateScore(doc.LinkID, float64(numDocs-i)); err != nil {
			t.Fatalf("could not update score: %v", err)
		}
	}

	it, err := idx.Search(index.Query{
		Type:       index.QueryTypeMatch,
		Expression: "poeta",
		Offset:     20,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	ids := iterateDocs(t, it)
	if !cmp.Equal(ids, expIDs[20:]) {
		t.Errorf("search returned incorrect IDs")
	}

	// Search with offset beyond the total number of results.
	it, err = idx.Search(index.Query{
		Type:       index.QueryTypeMatch,
		Expression: "poeta",
		Offset:     200,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	ids = iterateDocs(t, it)
	if len(ids) != 0 {
		t.Errorf("got %d IDs, want %d", len(ids), 0)
	}
}

// TestUpdateScore checks that PageRank score updates work as expected.
func TestUpdateScore(t *testing.T, idx index.Indexer) {
	var (
		numDocs = 100
		expIDs  []uuid.UUID
	)
	for i := 0; i < numDocs; i++ {
		id := uuid.New()
		expIDs = append(expIDs, id)
		doc := &index.Document{
			LinkID:  id,
			Title:   fmt.Sprintf("doc with ID %s", id.String()),
			Content: "Ovidius poeta in terra pontica",
		}

		if err := idx.Index(doc); err != nil {
			t.Fatalf("could not index document: %v", err)
		}

		if err := idx.UpdateScore(doc.LinkID, float64(numDocs-i)); err != nil {
			t.Fatalf("could not update score: %v", err)
		}
	}

	it, err := idx.Search(index.Query{
		Type:       index.QueryTypeMatch,
		Expression: "poeta",
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	ids := iterateDocs(t, it)
	if !cmp.Equal(ids, expIDs) {
		t.Errorf("search returned incorrect IDs")
	}

	// Update the pagerank scores so that results are sorted in the
	// reverse order.
	for i := 0; i < numDocs; i++ {
		if err = idx.UpdateScore(expIDs[i], float64(i)); err != nil {
			t.Fatalf("could not update score for %s: %v", expIDs[i].String(), err)
		}
	}

	it, err = idx.Search(index.Query{
		Type:       index.QueryTypeMatch,
		Expression: "poeta",
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	ids = iterateDocs(t, it)
	if !cmp.Equal(ids, reverse(expIDs)) {
		t.Errorf("search returned incorrect IDs")
	}
}

// TestUpdateScoreForUnknownDocument checks that a placeholder document will
// be created when setting the PageRank score for an unknown document.
func TestUpdateScoreForUnknownDocument(t *testing.T, idx index.Indexer) {
	linkID := uuid.New()
	if err := idx.UpdateScore(linkID, 0.5); err != nil {
		t.Fatalf("could not update score: %v", err)
	}

	doc, err := idx.FindByID(linkID)
	if err != nil {
		t.Fatalf("could not find document: %v", err)
	}

	if doc.URL != "" {
		t.Errorf("url = %q, want %q", doc.URL, "")
	}
	if doc.Title != "" {
		t.Errorf("title = %q, want %q", doc.Title, "")
	}
	if doc.Content != "" {
		t.Errorf("title = %q, want %q", doc.Content, "")
	}
	if !doc.IndexedAt.IsZero() {
		t.Errorf("non-zero timestamp %v", doc.IndexedAt)
	}
	if doc.PageRank != 0.5 {
		t.Errorf("pagerank score = %f, want %f", doc.PageRank, 0.5)
	}
}

func iterateDocs(t *testing.T, it index.Iterator) []uuid.UUID {
	var seen []uuid.UUID
	for it.Next() {
		seen = append(seen, it.Document().LinkID)
	}
	if err := it.Error(); err != nil {
		t.Fatalf("iterator error: %v", err)
	}
	if err := it.Close(); err != nil {
		t.Fatalf("failed to close iterator: %v", err)
	}
	return seen
}

func reverse(in []uuid.UUID) []uuid.UUID {
	for left, right := 0, len(in)-1; left < right; left, right = left+1, right-1 {
		in[left], in[right] = in[right], in[left]
	}

	return in
}
