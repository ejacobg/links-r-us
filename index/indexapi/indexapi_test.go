package indexapi_test

import (
	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/protobuf/types/known/timestamppb"
	gc "gopkg.in/check.v1"
	"testing"
	"time"
)

func Test(t *testing.T) {
	// Run all gocheck test-suites
	gc.TestingT(t)
}

func mustEncodeTimestamp(c *gc.C, t time.Time) *timestamp.Timestamp {
	ts := timestamppb.New(t)
	c.Assert(ts.CheckValid(), gc.IsNil)
	return ts
}
