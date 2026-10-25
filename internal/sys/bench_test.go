package sys

import (
	"testing"
	"time"
)

func BenchmarkSample(b *testing.B) {
	exes := NewExeCache()
	s := NewSampler(exes, NewResolver(exes, 4))
	s.Sample(time.Now())
	s.WaitResolved(2 * time.Second)
	s.Sample(time.Now())
	b.ReportAllocs()
	for b.Loop() {
		s.Sample(time.Now())
	}
}
