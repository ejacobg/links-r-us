package graphtest

import (
	"errors"
	"fmt"
	"github.com/ejacobg/links-r-us/graph"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"math/big"
	"sort"
	"sync"
	"testing"
	"time"
)

// Suite defines a re-usable set of graph-related tests that can
// be executed against any type that implements graph.Graph.
type Suite struct {
	G graph.Graph

	// Optional helper functions.
	BeforeEach func(*testing.T)
	AfterEach  func(*testing.T)
}

func (s *Suite) TestGraph(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*testing.T, graph.Graph)
	}{
		{"Upsert link", TestUpsertLink},
		{"Find link", TestFindLink},
		{"Concurrent link iterators", TestConcurrentLinkIterators},
		{"Link iterator time filter", TestLinkIteratorTimeFilter},
		{"Partitioned link iterators", TestPartitionedLinkIterators},
		{"Upsert edge", TestUpsertEdge},
		{"Concurrent edge iterators", TestConcurrentEdgeIterators},
		{"Edge iterator time filter", TestEdgeIteratorTimeFilter},
		{"Partitioned edge iterators", TestPartitionedEdgeIterators},
		{"Remove stale edges", TestRemoveStaleEdges},
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
			test.fn(t, s.G)
			s.AfterEach(t)
		})
	}
}

// TestUpsertLink verifies the link upsert logic.
func TestUpsertLink(t *testing.T, g graph.Graph) {
	// Create a new link.
	original := &graph.Link{
		URL:         "https://example.com",
		RetrievedAt: time.Now().Add(-10 * time.Hour),
	}
	if err := g.UpsertLink(original); err != nil {
		t.Fatalf("failed to insert link: %v", err)
	}
	if original.ID == uuid.Nil {
		t.Fatalf("expected a linkID to be assigned to the new link")
	}

	// Update existing link with a newer timestamp and different URL.
	accessedAt := time.Now().Truncate(time.Second).UTC()
	updated := &graph.Link{
		ID:          original.ID,
		URL:         "https://example.com",
		RetrievedAt: accessedAt,
	}
	if err := g.UpsertLink(updated); err != nil {
		t.Fatalf("failed to insert link: %v", err)
	}
	if updated.ID != original.ID {
		t.Fatalf("link ID changed while upserting")
	}

	stored, err := g.FindLink(updated.ID)
	if err != nil {
		t.Fatalf("could not find link: %v", err)
	}
	if stored.RetrievedAt != accessedAt {
		t.Errorf("last accessed timestamp was not updated")
	}

	// Attempt to insert a new link whose URL matches an existing link
	// and provide an older accessedAt value.
	sameURL := &graph.Link{
		URL:         updated.URL,
		RetrievedAt: time.Now().Add(-10 * time.Hour).UTC(),
	}
	if err = g.UpsertLink(sameURL); err != nil {
		t.Fatalf("failed to update link: %v", err)
	}
	if sameURL.ID != updated.ID {
		t.Fatalf("link ID changed while upserting")
	}

	stored, err = g.FindLink(updated.ID)
	if err != nil {
		t.Fatalf("could not find link: %v", err)
	}
	if stored.RetrievedAt != accessedAt {
		t.Errorf("last accessed timestamp was overwritten with an older value")
	}

	// Create a new link and then attempt to update its URL to the same as
	// an existing link.
	dup := &graph.Link{
		URL: "foo",
	}
	if err = g.UpsertLink(dup); err != nil {
		t.Fatalf("failed to update link: %v", err)
	}
	if dup.ID == uuid.Nil {
		t.Errorf("expected a linkID to be assigned to the new link")
	}
}

// TestFindLink verifies the link lookup logic.
func TestFindLink(t *testing.T, g graph.Graph) {
	// Create a new link.
	link := &graph.Link{
		URL:         "https://example.com",
		RetrievedAt: time.Now().Truncate(time.Second).UTC(),
	}
	if err := g.UpsertLink(link); err != nil {
		t.Fatalf("failed to insert link: %v", err)
	}
	if link.ID == uuid.Nil {
		t.Errorf("expected a linkID to be assigned to the new link")
	}

	// Look up link by ID.
	other, err := g.FindLink(link.ID)
	if err != nil {
		t.Fatalf("could not find link: %v", err)
	}
	if !cmp.Equal(other, link) {
		t.Errorf("lookup by ID returned the wrong link")
	}

	// Look up link by unknown ID.
	_, err = g.FindLink(uuid.Nil)
	if !errors.Is(err, graph.ErrNotFound) {
		t.Errorf("unexpected error %v, want %v", err, graph.ErrNotFound)
	}
}

// TestConcurrentLinkIterators verifies that multiple clients can concurrently
// access the store.
func TestConcurrentLinkIterators(t *testing.T, g graph.Graph) {
	var (
		wg           sync.WaitGroup
		numIterators = 10
		numLinks     = 100
	)

	for i := 0; i < numLinks; i++ {
		link := &graph.Link{URL: fmt.Sprint(i)}
		if err := g.UpsertLink(link); err != nil {
			t.Fatalf("failed to insert link: %v", err)
		}
	}

	wg.Add(numIterators)
	for i := 0; i < numIterators; i++ {
		go func(id int) {
			defer wg.Done()

			itTagComment := fmt.Sprintf("iterator %d", id)
			it, err := partitionedLinkIterator(t, g, 0, 1, time.Now())
			if err != nil {
				t.Errorf("%s: failed to gather links: %v", itTagComment, err)
				return
			}
			defer func() {
				if err := it.Close(); err != nil {
					t.Errorf("failed to close iterator: %v", err)
				}
			}()

			seen := make(map[string]bool)
			for i := 0; it.Next(); i++ {
				link := it.Link()
				linkID := link.ID.String()
				if seen[linkID] {
					t.Errorf("%s saw the same link twice", itTagComment)
				}
				seen[linkID] = true
			}

			if len(seen) != numLinks {
				t.Errorf("%s returns %d links, want %d", itTagComment, len(seen), numLinks)
			}
			if err = it.Error(); err != nil {
				t.Errorf("%s error: %v", itTagComment, err)
			}
			if err = it.Close(); err != nil {
				t.Errorf("%s failed to close %v", itTagComment, err)
			}
		}(i)
	}

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
	// test completed successfully
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for test to complete")
	}
}

// TestLinkIteratorTimeFilter verifies that the time-based filtering of the
// link iterator works as expected.
func TestLinkIteratorTimeFilter(t *testing.T, g graph.Graph) {
	linkUUIDs := make([]uuid.UUID, 3)
	linkInsertTimes := make([]time.Time, len(linkUUIDs))
	for i := 0; i < len(linkUUIDs); i++ {
		link := &graph.Link{URL: fmt.Sprint(i), RetrievedAt: time.Now()}
		if err := g.UpsertLink(link); err != nil {
			t.Fatalf("failed to insert link: %v", err)
		}
		linkUUIDs[i] = link.ID
		time.Sleep(time.Millisecond)
		linkInsertTimes[i] = time.Now()
	}

	for i, ts := range linkInsertTimes {
		t.Logf("fetching links created before edge %d", i)
		assertIteratedLinkIDsMatch(t, g, ts, linkUUIDs[:i+1])
	}
}

func assertIteratedLinkIDsMatch(t *testing.T, g graph.Graph, updatedBefore time.Time, exp []uuid.UUID) {
	it, err := partitionedLinkIterator(t, g, 0, 1, updatedBefore)
	if err != nil {
		t.Fatalf("failed to create iterator: %v", err)
	}

	var got []uuid.UUID
	for it.Next() {
		got = append(got, it.Link().ID)
	}
	if err = it.Error(); err != nil {
		t.Errorf("iterator error: %v", err)
	}
	if err = it.Close(); err != nil {
		t.Errorf("failed to close iterator: %v", err)
	}

	sort.Slice(got, func(l, r int) bool { return got[l].String() < got[r].String() })
	sort.Slice(exp, func(l, r int) bool { return exp[l].String() < exp[r].String() })
	if !cmp.Equal(got, exp) {
		t.Errorf("iterated IDs do not match")
		t.Log("got:")
		for _, u := range got {
			t.Logf("%s\n", u.String())
		}
		t.Log("want:")
		for _, u := range exp {
			t.Logf("%s\n", u.String())
		}
	}
}

// TestPartitionedLinkIterators verifies that the graph partitioning logic
// works as expected even when partitions contain an uneven number of items.
func TestPartitionedLinkIterators(t *testing.T, g graph.Graph) {
	numLinks := 100
	numPartitions := 10
	for i := 0; i < numLinks; i++ {
		if err := g.UpsertLink(&graph.Link{URL: fmt.Sprint(i)}); err != nil {
			t.Fatalf("failed to insert link: %v", err)
		}
	}

	// Check with both odd and even partition counts to check for rounding-related bugs.
	count := iteratePartitionedLinks(t, g, numPartitions)
	if count != numLinks {
		t.Errorf("got %d links, want %d", count, numLinks)
	}
	count = iteratePartitionedLinks(t, g, numPartitions+1)
	if count != numLinks {
		t.Errorf("got %d links, want %d", count, numLinks)
	}
}

func iteratePartitionedLinks(t *testing.T, g graph.Graph, numPartitions int) int {
	seen := make(map[string]bool)
	for partition := 0; partition < numPartitions; partition++ {
		it, err := partitionedLinkIterator(t, g, partition, numPartitions, time.Now())
		if err != nil {
			t.Fatalf("failed to create iterator: %v", err)
		}
		defer func() {
			if err = it.Close(); err != nil {
				t.Errorf("failed to close iterator: %v", err)
			}
		}()

		for it.Next() {
			link := it.Link()
			linkID := link.ID.String()
			if seen[linkID] {
				t.Error("iterator returned same link in different partitions")
			}
			seen[linkID] = true
		}

		if err = it.Error(); err != nil {
			t.Errorf("iterator error: %v", err)
		}
		if err = it.Close(); err != nil {
			t.Errorf("failed to close iterator: %v", err)
		}
	}

	return len(seen)
}

// TestUpsertEdge verifies the edge upsert logic.
func TestUpsertEdge(t *testing.T, g graph.Graph) {
	// Create links.
	linkUUIDs := make([]uuid.UUID, 3)
	for i := 0; i < 3; i++ {
		link := &graph.Link{URL: fmt.Sprint(i)}
		if err := g.UpsertLink(link); err != nil {
			t.Fatalf("failed to insert link: %v", err)
		}
		linkUUIDs[i] = link.ID
	}

	// Create an edge.
	edge := &graph.Edge{
		Src: linkUUIDs[0],
		Dst: linkUUIDs[1],
	}

	err := g.UpsertEdge(edge)
	if err != nil {
		t.Fatalf("failed to insert edge: %v", err)
	}
	if edge.ID == uuid.Nil {
		t.Fatalf("expected an edgeID to be assigned to the new edge")
	}
	if edge.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt field not set")
	}

	time.Sleep(time.Millisecond)

	// Update existing edge.
	other := &graph.Edge{
		ID:  edge.ID,
		Src: linkUUIDs[0],
		Dst: linkUUIDs[1],
	}
	err = g.UpsertEdge(other)
	if err != nil {
		t.Fatalf("failed to update edge: %v", err)
	}
	if other.ID != edge.ID {
		t.Errorf("edge ID changed while upserting")
	}
	if other.UpdatedAt == edge.UpdatedAt {
		t.Errorf("UpdatedAt field not modified")
	}

	// Create edge with unknown link IDs.
	bogus := &graph.Edge{
		Src: linkUUIDs[0],
		Dst: uuid.New(),
	}
	err = g.UpsertEdge(bogus)
	if !errors.Is(err, graph.ErrUnknownEdgeLinks) {
		t.Errorf("unexpected error %v, want %v", err, graph.ErrUnknownEdgeLinks)
	}
}

// TestConcurrentEdgeIterators verifies that multiple clients can concurrently
// access the store.
func TestConcurrentEdgeIterators(t *testing.T, g graph.Graph) {
	var (
		wg           sync.WaitGroup
		numIterators = 10
		numEdges     = 100
		linkUUIDs    = make([]uuid.UUID, numEdges*2)
	)

	for i := 0; i < numEdges*2; i++ {
		link := &graph.Link{URL: fmt.Sprint(i)}
		if err := g.UpsertLink(link); err != nil {
			t.Fatalf("failed to insert link: %v", err)
		}
		linkUUIDs[i] = link.ID
	}
	for i := 0; i < numEdges; i++ { // Why make 200 links in the above loop when you're only going to use 100?
		if err := g.UpsertEdge(&graph.Edge{
			Src: linkUUIDs[0],
			Dst: linkUUIDs[i],
		}); err != nil {
			t.Fatalf("failed to insert edge: %v", err)
		}
	}

	time.Sleep(time.Millisecond)

	wg.Add(numIterators)
	for i := 0; i < numIterators; i++ {
		go func(id int) {
			defer wg.Done()

			itTagComment := fmt.Sprintf("iterator %d", id)
			it, err := partitionedEdgeIterator(t, g, 0, 1, time.Now())
			if err != nil {
				t.Errorf("%s: failed to gather links: %v", itTagComment, err)
				return
			}
			defer func() {
				if err := it.Close(); err != nil {
					t.Errorf("failed to close iterator: %v", err)
				}
			}()

			seen := make(map[string]bool)
			for i := 0; it.Next(); i++ {
				edge := it.Edge()
				edgeID := edge.ID.String()
				if seen[edgeID] {
					t.Errorf("%s saw the same link twice", itTagComment)
				}
				seen[edgeID] = true
			}

			if len(seen) != numEdges {
				t.Errorf("%s returns %d links, want %d", itTagComment, len(seen), numEdges)
			}
			if err = it.Error(); err != nil {
				t.Errorf("%s error: %v", itTagComment, err)
			}
			if err = it.Close(); err != nil {
				t.Errorf("%s failed to close: %v", itTagComment, err)
			}
		}(i)
	}

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
	// test completed successfully
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for test to complete")
	}
}

// TestEdgeIteratorTimeFilter verifies that the time-based filtering of the
// edge iterator works as expected.
func TestEdgeIteratorTimeFilter(t *testing.T, g graph.Graph) {
	linkUUIDs := make([]uuid.UUID, 3)
	for i := 0; i < len(linkUUIDs); i++ {
		link := &graph.Link{URL: fmt.Sprint(i)}
		if err := g.UpsertLink(link); err != nil {
			t.Fatalf("failed to insert link: %v", err)
		}
		linkUUIDs[i] = link.ID
	}

	edgeUUIDs := make([]uuid.UUID, len(linkUUIDs))
	edgeInsertTimes := make([]time.Time, len(linkUUIDs))
	for i := 0; i < len(linkUUIDs); i++ {
		edge := &graph.Edge{Src: linkUUIDs[0], Dst: linkUUIDs[i]}
		if err := g.UpsertEdge(edge); err != nil {
			t.Fatalf("failed to insert edge: %v", err)
		}
		edgeUUIDs[i] = edge.ID
		time.Sleep(time.Millisecond)
		edgeInsertTimes[i] = time.Now()
	}

	for i, ts := range edgeInsertTimes {
		t.Logf("fetching edges created before edge %d", i)
		assertIteratedEdgeIDsMatch(t, g, ts, edgeUUIDs[:i+1])
	}
}

func assertIteratedEdgeIDsMatch(t *testing.T, g graph.Graph, updatedBefore time.Time, exp []uuid.UUID) {
	it, err := partitionedEdgeIterator(t, g, 0, 1, updatedBefore)
	if err != nil {
		t.Fatalf("failed to create iterator: %v", err)
	}

	var got []uuid.UUID
	for it.Next() {
		got = append(got, it.Edge().ID)
	}
	if err = it.Error(); err != nil {
		t.Errorf("iterator error: %v", err)
	}
	if err = it.Close(); err != nil {
		t.Errorf("failed to close iterator: %v", err)
	}

	sort.Slice(got, func(l, r int) bool { return got[l].String() < got[r].String() })
	sort.Slice(exp, func(l, r int) bool { return exp[l].String() < exp[r].String() })
	if !cmp.Equal(got, exp) {
		t.Errorf("iterated IDs do not match")
	}
}

// TestPartitionedEdgeIterators verifies that the graph partitioning logic
// works as expected even when partitions contain an uneven number of items.
func TestPartitionedEdgeIterators(t *testing.T, g graph.Graph) {
	numEdges := 100
	numPartitions := 10
	linkUUIDs := make([]uuid.UUID, numEdges*2)
	for i := 0; i < numEdges*2; i++ {
		link := &graph.Link{URL: fmt.Sprint(i)}
		if err := g.UpsertLink(link); err != nil {
			t.Fatalf("failed to insert link: %v", err)
		}
		linkUUIDs[i] = link.ID
	}
	for i := 0; i < numEdges; i++ {
		if err := g.UpsertEdge(&graph.Edge{
			Src: linkUUIDs[0],
			Dst: linkUUIDs[i],
		}); err != nil {
			t.Fatalf("failed to insert edge: %v", err)
		}
	}

	// Check with both odd and even partition counts to check for rounding-related bugs.
	count := iteratePartitionedEdges(t, g, numPartitions)
	if count != numEdges {
		t.Errorf("got %d edges, want %d", count, numEdges)
	}
	count = iteratePartitionedEdges(t, g, numPartitions+1)
	if count != numEdges {
		t.Errorf("got %d edges, want %d", count, numEdges)
	}
}

func iteratePartitionedEdges(t *testing.T, g graph.Graph, numPartitions int) int {
	seen := make(map[string]bool)
	for partition := 0; partition < numPartitions; partition++ {
		// Build list of expected edges per partition. An edge belongs to a
		// partition if its origin link also belongs to the same partition.
		linksInPartition := make(map[uuid.UUID]struct{})
		linkIt, err := partitionedLinkIterator(t, g, partition, numPartitions, time.Now())
		if err != nil {
			t.Fatalf("failed to create link iterator: %v", err)
		}
		for linkIt.Next() {
			linkID := linkIt.Link().ID
			linksInPartition[linkID] = struct{}{}
		}

		it, err := partitionedEdgeIterator(t, g, partition, numPartitions, time.Now())
		if err != nil {
			t.Fatalf("failed to create edge iterator: %v", err)
		}
		defer func() {
			if err = it.Close(); err != nil {
				t.Errorf("failed to close edge iterator: %v", err)
			}
		}()

		for it.Next() {
			edge := it.Edge()
			edgeID := edge.ID.String()
			if seen[edgeID] {
				t.Error("iterator returned same edge in different partitions")
			}
			seen[edgeID] = true

			_, srcInPartition := linksInPartition[edge.Src]
			if !srcInPartition {
				t.Error("iterator returned an edge whose source link belongs to a different partition")
			}
		}

		if err = it.Error(); err != nil {
			t.Errorf("edge iterator error: %v", err)
		}
		if err = it.Close(); err != nil {
			t.Errorf("failed to close edge iterator: %v", err)
		}
	}

	return len(seen)
}

// TestRemoveStaleEdges verifies that the edge deletion logic works as expected.
func TestRemoveStaleEdges(t *testing.T, g graph.Graph) {
	numEdges := 100
	linkUUIDs := make([]uuid.UUID, numEdges*4)
	goneUUIDs := make(map[uuid.UUID]struct{})

	// Add links to the graph.
	for i := 0; i < numEdges*4; i++ {
		link := &graph.Link{URL: fmt.Sprint(i)}
		if err := g.UpsertLink(link); err != nil {
			t.Fatalf("failed to insert link: %v", err)
		}
		// Record the ID of each link that we add.
		linkUUIDs[i] = link.ID
	}

	// Add all of our edges. All added edges will have link[0] as the source.
	var lastTs time.Time
	for i := 0; i < numEdges; i++ {
		e1 := &graph.Edge{
			Src: linkUUIDs[0],
			Dst: linkUUIDs[i],
		}
		if err := g.UpsertEdge(e1); err != nil {
			t.Fatalf("failed to insert edge: %v", err)
		}
		// Record the ID of each edge that we add. All the edges in this batch are expected to be removed, invalidating all of these IDs.
		goneUUIDs[e1.ID] = struct{}{}
		// Record the timestamp of the last edge that was added.
		lastTs = e1.UpdatedAt
	}

	// Record a timestamp that is after the last edge that was added in the first batch.
	deleteBefore := lastTs.Add(time.Millisecond)

	// Use a small delay to guarantee that all edges in the second batch are after the recorded timestamp.
	time.Sleep(time.Millisecond)

	// The following edges will have an updated at value > lastTs
	for i := 0; i < numEdges; i++ {
		e2 := &graph.Edge{
			Src: linkUUIDs[0],
			Dst: linkUUIDs[numEdges+i+1],
		}
		if err := g.UpsertEdge(e2); err != nil {
			t.Fatalf("failed to insert edge: %v", err)
		}
	}

	// This should remove all the edges from the first batch.
	if err := g.RemoveStaleEdges(linkUUIDs[0], deleteBefore); err != nil {
		t.Errorf("failed to remove stale edges: %v", err)
	}

	// Use a small delay to guarantee that we see all the current edges.
	time.Sleep(time.Millisecond)

	it, err := partitionedEdgeIterator(t, g, 0, 1, time.Now())
	if err != nil {
		t.Fatalf("failed to create iterator: %v", err)
	}
	defer func() {
		if err = it.Close(); err != nil {
			t.Errorf("failed to close iterator: %v", err)
		}
	}()

	var seen int
	for it.Next() {
		id := it.Edge().ID
		_, found := goneUUIDs[id]
		if found {
			t.Errorf("expected edge %s to be removed from the edge list", id.String())
		}
		seen++
	}

	if seen != numEdges {
		t.Errorf("saw %d edges, want %d", seen, numEdges)
	}
}

func partitionedLinkIterator(t *testing.T, g graph.Graph, partition, numPartitions int, accessedBefore time.Time) (graph.LinkIterator, error) {
	from, to := partitionRange(t, partition, numPartitions)
	return g.Links(from, to, accessedBefore)
}

func partitionedEdgeIterator(t *testing.T, g graph.Graph, partition, numPartitions int, updatedBefore time.Time) (graph.EdgeIterator, error) {
	from, to := partitionRange(t, partition, numPartitions)
	return g.Edges(from, to, updatedBefore)
}

func partitionRange(t *testing.T, partition, numPartitions int) (from, to uuid.UUID) {
	if partition < 0 || partition >= numPartitions {
		t.Fatal("invalid partition")
	}

	var minUUID = uuid.Nil
	var maxUUID = uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")
	var err error

	// Calculate the size of each partition as: (2^128 / numPartitions)
	tokenRange := big.NewInt(0)
	partSize := big.NewInt(0)
	partSize.SetBytes(maxUUID[:])
	partSize = partSize.Div(partSize, big.NewInt(int64(numPartitions)))

	// We model the partitions as a segment that begins at minUUID (all
	// bits set to zero) and ends at maxUUID (all bits set to 1). By
	// setting the end range for the *last* partition to maxUUID we ensure
	// that we always cover the full range of UUIDs even if the range
	// itself is not evenly divisible by numPartitions.
	if partition == 0 {
		from = minUUID
	} else {
		tokenRange.Mul(partSize, big.NewInt(int64(partition)))
		from, err = uuid.FromBytes(tokenRange.Bytes())
		if err != nil {
			t.Fatalf("failed to create UUID: %v", err)
		}
	}

	if partition == numPartitions-1 {
		to = maxUUID
	} else {
		tokenRange.Mul(partSize, big.NewInt(int64(partition+1)))
		to, err = uuid.FromBytes(tokenRange.Bytes())
		if err != nil {
			t.Fatalf("failed to create UUID: %v", err)
		}
	}

	return from, to
}
