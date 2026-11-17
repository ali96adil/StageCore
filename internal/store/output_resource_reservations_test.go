package store

import (
	"strconv"
	"sync/atomic"
	"testing"
)

func BenchmarkOutputResourceReservation(b *testing.B) {
	resources := []string{"device:dmx-1", "osc:stagecore.local:9000", "group:actors"}
	for _, parallel := range []bool{false, true} {
		name := "Sequential"
		if parallel {
			name = "ParallelSharedOutput"
		}
		b.Run(name, func(b *testing.B) {
			st := New(nil, nil)
			var sequence atomic.Uint64
			b.ReportAllocs()
			if parallel {
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						owner := strconv.FormatUint(sequence.Add(1), 10)
						release, conflict := st.ReserveOutputResources(owner, "cue-"+owner, resources)
						if conflict == "" {
							release()
						}
					}
				})
				return
			}
			for i := 0; i < b.N; i++ {
				owner := strconv.FormatUint(sequence.Add(1), 10)
				release, conflict := st.ReserveOutputResources(owner, "cue-"+owner, resources)
				if conflict != "" {
					b.Fatalf("sequential reservation conflicted with %s", conflict)
				}
				release()
			}
		})
	}
}
