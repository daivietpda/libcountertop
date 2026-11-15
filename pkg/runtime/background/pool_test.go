package background

import (
	"sync/atomic"
	"testing"
)

func TestSplitCoversEveryIndexOnce(t *testing.T) {
	p := New(2, 0)
	defer p.Close()
	for _, n := range []int{0, 1, 2, 3, 10, 1001} {
		hits := make([]atomic.Int32, n)
		p.Split(n, func(lo, hi int) {
			for i := lo; i < hi; i++ {
				hits[i].Add(1)
			}
		})
		for i := range hits {
			if got := hits[i].Load(); got != 1 {
				t.Fatalf("n=%d index %d covered %d times", n, i, got)
			}
		}
	}
}

func TestRunWaitsForTheJob(t *testing.T) {
	p := New(0, 0)
	defer p.Close()
	ran := false
	p.Run(func() { ran = true })
	if !ran {
		t.Fatal("Run returned before the job ran")
	}
}
