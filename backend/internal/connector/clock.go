package connector

import "time"

// RealClock delegates timers to the Go runtime.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

func (RealClock) AfterFunc(d time.Duration, f func()) func() bool {
	timer := time.AfterFunc(d, f)
	return timer.Stop
}
