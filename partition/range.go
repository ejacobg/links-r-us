package partition

import (
	"bytes"
	"fmt"
	"github.com/google/uuid"
	"math/big" // Go does not have built-in support 128-bit arithmetic.
)

// Range represents a contiguous UUID region which is split into a number of
// partitions.
type Range struct {
	// The lower bound for the range (inclusive).
	start uuid.UUID

	// rangeSplits[i] holds the upper bound (exclusive) for partition i.
	// Each partition (i) looks like this: [rangeSplits[i-1], rangeSplits[i]).
	// The lower bound for the first partition is always start, and the upper bound for the last partition is always the maximum UUID.
	rangeSplits []uuid.UUID
}

// NewRange creates a new range [start, end) and splits it into the
// provided number of partitions.
func NewRange(start, end uuid.UUID, numPartitions int) (Range, error) {
	// Verify that start is less than end, and that we can generate the number of partitions.
	if bytes.Compare(start[:], end[:]) >= 0 {
		return Range{}, fmt.Errorf("range start UUID must be less than the end UUID")
	} else if numPartitions <= 0 {
		return Range{}, fmt.Errorf("number of partitions must be at least equal to 1")
	}

	// Calculate the size of each partition as: ((end - start + 1) / numPartitions)
	partSize := big.NewInt(0)
	partSize = partSize.Sub(big.NewInt(0).SetBytes(end[:]), big.NewInt(0).SetBytes(start[:]))
	partSize = partSize.Div(partSize.Add(partSize, big.NewInt(1)), big.NewInt(int64(numPartitions)))

	var (
		to         uuid.UUID
		err        error
		ranges     = make([]uuid.UUID, numPartitions)
		tokenRange = big.NewInt(0)
	)
	for partition := 0; partition < numPartitions-1; partition++ {
		// The upper bound is the start of the next bound (partition+1).
		tokenRange.Mul(partSize, big.NewInt(int64(partition+1)))
		if to, err = uuid.FromBytes(tokenRange.Bytes()); err != nil {
			return Range{}, fmt.Errorf("partition range: %w", err)
		}

		ranges[partition] = to
	}
	// The last partition will always extend to the end of the range.
	ranges[numPartitions-1] = end

	return Range{start: start, rangeSplits: ranges}, nil
}

// NewFullRange creates a new range that uses the full UUID value space and
// splits it into the provided number of partitions.
func NewFullRange(numPartitions int) (Range, error) {
	return NewRange(
		uuid.Nil,
		uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff"),
		numPartitions,
	)
}

// Extents returns the full [start, end) range this object represents.
func (r Range) Extents() (uuid.UUID, uuid.UUID) {
	return r.start, r.rangeSplits[len(r.rangeSplits)-1]
}

// PartitionExtents returns the [start, end) range for the requested partition.
func (r Range) PartitionExtents(partition int) (uuid.UUID, uuid.UUID, error) {
	if partition < 0 || partition >= len(r.rangeSplits) {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid partition index")
	}

	if partition == 0 {
		return r.start, r.rangeSplits[0], nil
	}
	return r.rangeSplits[partition-1], r.rangeSplits[partition], nil
}
