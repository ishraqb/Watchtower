// Package anomaly watches the live tick stream and flags unusual volume spikes.
// The idea is simple: keep a short rolling window of recent volume per symbol
// and shout when a tick blows way past the recent average.
package anomaly

import (
	"sync"

	"github.com/ishraqb/Watchtower/backend/internal/db"
)

const (
	// WindowSize is the number of recent ticks used for the rolling average.
	WindowSize = 10
	// Threshold is the multiple of the rolling average that triggers an anomaly.
	Threshold = 3.0
)

// Detection describes a fired volume anomaly.
type Detection struct {
	Symbol        string
	TriggerVolume int
	AvgVolume     float64
}

// ringBuffer is a fixed-size circular buffer of recent volumes for one symbol.
type ringBuffer struct {
	data   []int
	size   int
	next   int
	filled bool
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{data: make([]int, size), size: size}
}

func (r *ringBuffer) add(v int) {
	r.data[r.next] = v
	r.next = (r.next + 1) % r.size
	if r.next == 0 {
		r.filled = true
	}
}

// average returns the mean of the buffered values and how many are present.
func (r *ringBuffer) average() (float64, int) {
	count := r.size
	if !r.filled {
		count = r.next
	}
	if count == 0 {
		return 0, 0
	}
	sum := 0
	for i := 0; i < count; i++ {
		sum += r.data[i]
	}
	return float64(sum) / float64(count), count
}

// Detector tracks per-symbol volume history and flags spikes.
// Safe for concurrent use; one tick stream calls Observe per tick.
type Detector struct {
	mu      sync.Mutex
	buffers map[string]*ringBuffer
}

// NewDetector creates an empty detector.
func NewDetector() *Detector {
	return &Detector{buffers: make(map[string]*ringBuffer)}
}

// Observe records a tick and returns a Detection (and true) when the tick's
// volume exceeds Threshold times the rolling average of a full window.
// The current tick is added AFTER the comparison so a spike isn't averaged
// against itself.
func (d *Detector) Observe(tick db.Tick) (Detection, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	buf, ok := d.buffers[tick.Symbol]
	if !ok {
		buf = newRingBuffer(WindowSize)
		d.buffers[tick.Symbol] = buf
	}

	avg, count := buf.average()
	buf.add(tick.Volume)

	// Only evaluate once we have a full window to avoid noisy early triggers.
	if count < WindowSize || avg <= 0 {
		return Detection{}, false
	}

	if float64(tick.Volume) > Threshold*avg {
		return Detection{
			Symbol:        tick.Symbol,
			TriggerVolume: tick.Volume,
			AvgVolume:     avg,
		}, true
	}
	return Detection{}, false
}
