package usbdmx

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const refreshIntervalSamples = 100

var refreshContentionSink atomic.Uint64

type refreshProbePort struct {
	frames chan time.Time
	closed atomic.Bool
}

func (p *refreshProbePort) WritePacket(ctx context.Context, packet [FrameSize]byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if packet != ZeroFrame() {
		select {
		case p.frames <- time.Now():
		default:
			return errors.New("usbdmx: refresh probe overflow")
		}
	}
	return nil
}

func (p *refreshProbePort) Close() error {
	p.closed.Store(true)
	return nil
}

// BenchmarkManualContinuousRefreshJitter measures the 250 ms continuous USB-DMX
// refresh loop under CPU contention. Packet writes go to an in-memory probe;
// no serial port, device discovery, or physical output is involved. Run with
// -cpu=1,4 and -benchtime=1x to collect 100 refresh intervals per setting.
func BenchmarkManualContinuousRefreshJitter(b *testing.B) {
	intervals := make([]time.Duration, 0, b.N*refreshIntervalSamples)
	var totalGC uint64
	var totalGCPause uint64
	b.ReportAllocs()
	b.ReportMetric(refreshIntervalSamples, "intervals/op")
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		b.StopTimer()
		ctx, cancel := context.WithCancel(context.Background())
		port := &refreshProbePort{frames: make(chan time.Time, refreshIntervalSamples+2)}
		output := NewManualOutput(true, func(context.Context) (PacketPort, error) { return port, nil })
		stopLoad := make(chan struct{})
		var workers sync.WaitGroup
		workerCount := max(1, runtime.GOMAXPROCS(0)-1)
		for range workerCount {
			workers.Add(1)
			go func() {
				defer workers.Done()
				work := uint64(1)
				for {
					select {
					case <-stopLoad:
						refreshContentionSink.Store(work)
						return
					default:
						work = work*1664525 + 1013904223
						if work&0xffff == 0 {
							refreshContentionSink.Store(work)
						}
					}
				}
			}()
		}
		done := make(chan error, 1)
		var before runtime.MemStats
		runtime.ReadMemStats(&before)
		b.StartTimer()
		go func() {
			done <- output.Run(ctx, ManualRequest{
				Channels: []ChannelValue{{Channel: 1, Value: 200}},
			}, allowDMX)
		}()
		timestamps := make([]time.Time, refreshIntervalSamples+1)
		deadline := time.NewTimer(time.Minute)
		for index := range timestamps {
			select {
			case timestamps[index] = <-port.frames:
			case err := <-done:
				deadline.Stop()
				cancel()
				close(stopLoad)
				workers.Wait()
				b.Fatalf("continuous output stopped after %d refresh frames: %v", index, err)
			case <-deadline.C:
				cancel()
				<-done
				close(stopLoad)
				workers.Wait()
				b.Fatal("continuous output did not produce 100 refresh intervals within one minute")
			}
		}
		cancel()
		err := <-done
		close(stopLoad)
		workers.Wait()
		deadline.Stop()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		b.StopTimer()
		if !errors.Is(err, context.Canceled) {
			b.Fatalf("continuous output cancellation = %v, want context.Canceled", err)
		}
		if !port.closed.Load() {
			b.Fatal("continuous output did not close the simulated port")
		}
		// The measured nonzero frames include the initial level and 100 refreshes.
		for index := 1; index < len(timestamps); index++ {
			intervals = append(intervals, timestamps[index].Sub(timestamps[index-1]))
		}
		totalGC += uint64(after.NumGC - before.NumGC)
		totalGCPause += after.PauseTotalNs - before.PauseTotalNs
	}
	b.StopTimer()
	sort.Slice(intervals, func(i, j int) bool { return intervals[i] < intervals[j] })
	median := durationPercentile(intervals, 0.50)
	for _, percentile := range []float64{0.50, 0.95, 0.99} {
		value := durationPercentile(intervals, percentile)
		suffix := fmt.Sprintf("p%d-ms", int(percentile*100))
		b.ReportMetric(float64(value)/float64(time.Millisecond), "refresh-"+suffix)
		jitter := durationPercentileAbsoluteDeviation(intervals, median, percentile)
		b.ReportMetric(float64(jitter)/float64(time.Millisecond), "jitter-"+suffix)
	}
	if b.N > 0 {
		b.ReportMetric(float64(totalGC)/float64(b.N), "gc/op")
		b.ReportMetric(float64(totalGCPause)/float64(b.N), "gc-pause-ns/op")
	}
}

func durationPercentile(values []time.Duration, percentile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values))*percentile+0.999999) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func durationPercentileAbsoluteDeviation(values []time.Duration, center time.Duration, percentile float64) time.Duration {
	deviations := make([]time.Duration, len(values))
	for index, value := range values {
		deviations[index] = value - center
		if deviations[index] < 0 {
			deviations[index] = -deviations[index]
		}
	}
	sort.Slice(deviations, func(i, j int) bool { return deviations[i] < deviations[j] })
	return durationPercentile(deviations, percentile)
}
