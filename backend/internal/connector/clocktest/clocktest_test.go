package clocktest

import (
	"reflect"
	"testing"
	"time"
)

func TestClockNowAndAdvance(t *testing.T) {
	start := time.Unix(100, 0)
	clock := New(start)
	if got := clock.Now(); !got.Equal(start) {
		t.Fatalf("initial Now() = %v, want %v", got, start)
	}
	clock.Advance(250 * time.Millisecond)
	if got, want := clock.Now(), start.Add(250*time.Millisecond); !got.Equal(want) {
		t.Errorf("Now() after Advance() = %v, want %v", got, want)
	}
	clock.Advance(-time.Second)
	if got, want := clock.Now(), start.Add(250*time.Millisecond); !got.Equal(want) {
		t.Errorf("negative Advance moved time backward: got %v, want %v", got, want)
	}
}

func TestAfterFuncOrderAndStop(t *testing.T) {
	clock := New(time.Unix(200, 0))
	fired := []string{}
	clock.AfterFunc(2*time.Second, func() { fired = append(fired, "later") })
	clock.AfterFunc(time.Second, func() { fired = append(fired, "first") })
	clock.AfterFunc(time.Second, func() { fired = append(fired, "second") })
	stop := clock.AfterFunc(time.Second, func() { fired = append(fired, "stopped") })
	if got := clock.Pending(); got != 4 {
		t.Fatalf("Pending() = %d, want 4", got)
	}
	if !stop() || stop() {
		t.Fatal("stop function should return true exactly once")
	}
	if got := clock.Pending(); got != 3 {
		t.Fatalf("Pending() after stopping a callback = %d, want 3", got)
	}
	clock.Advance(2 * time.Second)
	if want := []string{"first", "second", "later"}; !reflect.DeepEqual(fired, want) {
		t.Errorf("callbacks = %v, want %v", fired, want)
	}
	if got := clock.Pending(); got != 0 {
		t.Errorf("Pending() after firing callbacks = %d, want 0", got)
	}
}

func TestCallbackCanScheduleDueCallback(t *testing.T) {
	clock := New(time.Unix(300, 0))
	fired := []string{}
	clock.AfterFunc(time.Second, func() {
		fired = append(fired, "outer")
		clock.AfterFunc(0, func() { fired = append(fired, "inner") })
	})
	clock.Advance(time.Second)
	if want := []string{"outer", "inner"}; !reflect.DeepEqual(fired, want) {
		t.Errorf("callbacks = %v, want %v", fired, want)
	}
}
