package anomaly

import (
	"testing"

	"github.com/ishraqb/Watchtower/backend/internal/db"
)

func tick(symbol string, volume int) db.Tick {
	return db.Tick{Symbol: symbol, Volume: volume}
}

func TestNoDetectionBeforeFullWindow(t *testing.T) {
	d := NewDetector()
	// Fill fewer than WindowSize ticks; nothing should fire even on a big value.
	for i := 0; i < WindowSize-1; i++ {
		if _, fired := d.Observe(tick("AAPL", 100)); fired {
			t.Fatalf("unexpected detection before full window at tick %d", i)
		}
	}
	if _, fired := d.Observe(tick("AAPL", 100000)); fired {
		t.Fatal("should not fire until a full window of history exists")
	}
}

func TestDetectsSpikeAboveThreshold(t *testing.T) {
	d := NewDetector()
	for i := 0; i < WindowSize; i++ {
		d.Observe(tick("TSLA", 100)) // establish avg ~100 over a full window
	}
	det, fired := d.Observe(tick("TSLA", 400)) // 4x avg > 3x threshold
	if !fired {
		t.Fatal("expected detection for 4x volume spike")
	}
	if det.Symbol != "TSLA" || det.TriggerVolume != 400 {
		t.Fatalf("unexpected detection payload: %+v", det)
	}
}

func TestNoDetectionWithinThreshold(t *testing.T) {
	d := NewDetector()
	for i := 0; i < WindowSize; i++ {
		d.Observe(tick("RIVN", 100))
	}
	if _, fired := d.Observe(tick("RIVN", 250)); fired { // 2.5x < 3x threshold
		t.Fatal("should not fire for a sub-threshold increase")
	}
}

func TestPerSymbolIsolation(t *testing.T) {
	d := NewDetector()
	for i := 0; i < WindowSize; i++ {
		d.Observe(tick("AAPL", 100))
	}
	// A first-ever tick for a different symbol must not fire.
	if _, fired := d.Observe(tick("NVDA", 99999)); fired {
		t.Fatal("a new symbol should not fire on its first tick")
	}
}
