package bsp_test

import (
	"fmt"
	"github.com/ejacobg/links-r-us/bsp"
	gc "gopkg.in/check.v1"
	"testing"
)

var _ = gc.Suite(new(InMemoryQueueTest))

type InMemoryQueueTest struct {
	q bsp.Queue
}

func TestInMemoryQueue(t *testing.T) {
	// Run all gocheck test-suites
	gc.TestingT(t)
}

func (s *InMemoryQueueTest) SetUpTest(c *gc.C) {
	s.q = bsp.NewInMemoryQueue()
}

func (s *InMemoryQueueTest) TearDownTest(c *gc.C) {
	c.Assert(s.q.Close(), gc.IsNil)
}

func (s *InMemoryQueueTest) TestEnqueueDequeue(c *gc.C) {
	for i := 0; i < 10; i++ {
		err := s.q.Enqueue(msg{payload: fmt.Sprint(i)})
		c.Assert(err, gc.IsNil)
	}
	c.Assert(s.q.PendingMessages(), gc.Equals, true)

	// We expect the messages to be dequeued in reverse order
	var (
		it        = s.q.Messages()
		processed int
	)
	for expNext := 9; it.Next(); expNext-- {
		got := it.Message().(msg).payload
		c.Assert(got, gc.Equals, fmt.Sprint(expNext))
		processed++
	}
	c.Assert(processed, gc.Equals, 10)
	c.Assert(it.Error(), gc.IsNil)
}

func (s *InMemoryQueueTest) TestDiscard(c *gc.C) {
	for i := 0; i < 10; i++ {
		err := s.q.Enqueue(msg{payload: fmt.Sprint(i)})
		c.Assert(err, gc.IsNil)
	}
	c.Assert(s.q.PendingMessages(), gc.Equals, true)
	c.Assert(s.q.DiscardMessages(), gc.IsNil)
	c.Assert(s.q.PendingMessages(), gc.Equals, false)
}

type msg struct {
	payload string
}

func (msg) Type() string { return "msg" }
