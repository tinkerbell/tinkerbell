package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type classified struct{ noAction, conflict, permanent bool }

func (classified) Error() string     { return "classified" }
func (c classified) NoAction() bool  { return c.noAction }
func (c classified) Conflict() bool  { return c.conflict }
func (c classified) Permanent() bool { return c.permanent }

func TestRetrierNext(t *testing.T) {
	r := Retry{PollInterval: 10 * time.Second, MaxInterval: 8 * time.Second}
	tests := map[string]struct {
		err    error
		lo, hi time.Duration // the delay must be in [lo, hi)
	}{
		"success":            {err: nil, lo: 0, hi: 1},
		"no action":          {err: classified{noAction: true}, lo: 5 * time.Second, hi: 15 * time.Second},
		"wrapped no action":  {err: fmt.Errorf("read: %w", classified{noAction: true}), lo: 5 * time.Second, hi: 15 * time.Second},
		"permanent":          {err: classified{permanent: true}, lo: 5 * time.Second, hi: 15 * time.Second},
		"conflict":           {err: classified{conflict: true}, lo: 0, hi: 500 * time.Millisecond},
		"error, first retry": {err: errors.New("unavailable"), lo: 0, hi: time.Second},
		"methods returning false are plain errors": {err: classified{}, lo: 0, hi: time.Second},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			for range 100 {
				rt := newRetrier(r)
				if got := rt.next(tt.err); got < tt.lo || got >= tt.hi {
					t.Fatalf("delay = %v, want in [%v, %v)", got, tt.lo, tt.hi)
				}
			}
		})
	}
}

func TestRetrierBackoffGrowsAndCaps(t *testing.T) {
	rt := newRetrier(Retry{MaxInterval: 8 * time.Second})
	err := errors.New("unavailable")
	// Ceilings for consecutive failures: 1s, 2s, 4s, 8s, then capped at 8s.
	for i, ceiling := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, 8 * time.Second} {
		if got := rt.next(err); got < 0 || got >= ceiling {
			t.Fatalf("failure %d: delay = %v, want in [0, %v)", i+1, got, ceiling)
		}
	}
	if rt.failures != 6 {
		t.Fatalf("failures = %d, want 6", rt.failures)
	}
}

func TestRetrierResetsOnServerResponse(t *testing.T) {
	for name, err := range map[string]error{
		"success":   nil,
		"no action": classified{noAction: true},
		"conflict":  classified{conflict: true},
		"permanent": classified{permanent: true},
	} {
		t.Run(name, func(t *testing.T) {
			rt := newRetrier(Retry{})
			rt.next(errors.New("unavailable"))
			rt.next(errors.New("unavailable"))
			rt.next(err)
			if rt.failures != 0 {
				t.Fatalf("failures = %d, want 0", rt.failures)
			}
		})
	}
}

func TestRetryDefaults(t *testing.T) {
	got := newRetrier(Retry{}).Retry
	want := Retry{PollInterval: DefaultPollInterval, MaxInterval: DefaultMaxInterval}
	if got != want {
		t.Fatalf("defaults = %+v, want %+v", got, want)
	}
}

func TestRetrierConflictCappedByMaxInterval(t *testing.T) {
	rt := newRetrier(Retry{MaxInterval: time.Millisecond})
	for range 100 {
		if got := rt.next(classified{conflict: true}); got >= time.Millisecond {
			t.Fatalf("delay = %v, want < %v", got, time.Millisecond)
		}
	}
}

func TestSleep(t *testing.T) {
	if !sleep(context.Background(), time.Millisecond) {
		t.Fatal("sleep returned false for an uncancelled context")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleep(ctx, time.Hour) {
		t.Fatal("sleep returned true for a cancelled context")
	}
	if sleep(ctx, 0) {
		t.Fatal("sleep(0) returned true for a cancelled context")
	}
}
