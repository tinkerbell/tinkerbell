package agent

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
)

const (
	// DefaultPollInterval is the default average delay between polls when no work is available.
	DefaultPollInterval = 5 * time.Second
	// DefaultMaxInterval is the default cap on the delay before retrying after an error or a conflict.
	DefaultMaxInterval = 30 * time.Second

	// baseInterval is the first exponential backoff interval after an error.
	baseInterval = time.Second
	// conflictInterval bounds the delay before retrying after a conflict.
	conflictInterval = 500 * time.Millisecond
)

// Retry is the Agent's retry policy. The Agent is the only component that retries;
// the Tink Server makes a single attempt per request.
// See docs/technical/designs/AGENT_SERVER_RETRY.md.
//
// Transports classify errors by implementing any of these methods:
//
//	NoAction() bool  // no work is available, poll again later
//	Conflict() bool  // a transient conflict, retry soon
//	Permanent() bool // retrying will not succeed
//
// Any other error is retried with exponential backoff.
type Retry struct {
	// PollInterval is the average delay between polls when no work is available.
	// Each delay is uniformly random in [PollInterval/2, 3*PollInterval/2).
	PollInterval time.Duration
	// MaxInterval caps the delay before retrying after an error or a conflict.
	MaxInterval time.Duration
}

// retrier tracks consecutive failures to compute the delay before the next attempt.
type retrier struct {
	Retry
	failures int
}

func newRetrier(r Retry) *retrier {
	if r.PollInterval <= 0 {
		r.PollInterval = DefaultPollInterval
	}
	if r.MaxInterval <= 0 {
		r.MaxInterval = DefaultMaxInterval
	}
	return &retrier{Retry: r}
}

// next records the result of an attempt and returns how long to wait before the next one.
// Successes, idle answers, conflicts, and permanent rejections reset the exponential backoff;
// only errors grow it.
func (r *retrier) next(err error) time.Duration {
	switch {
	case err == nil:
		r.failures = 0
		return 0
	case isNoAction(err), isPermanent(err):
		// A rejected report means the server won't accept it however often it is sent, so go back to polling.
		r.failures = 0
		return between(r.PollInterval/2, r.PollInterval*3/2)
	case isConflict(err):
		r.failures = 0
		return between(0, min(conflictInterval, r.MaxInterval))
	default:
		d := r.backoff()
		r.failures++
		return d
	}
}

// backoff returns a full jitter exponential delay: uniformly random in [0, min(MaxInterval, baseInterval*2^failures)).
func (r *retrier) backoff() time.Duration {
	ceiling := baseInterval
	for i := 0; i < r.failures && ceiling < r.MaxInterval; i++ {
		ceiling *= 2
	}
	return between(0, min(ceiling, r.MaxInterval))
}

// between returns a uniformly random duration in [lo, hi), or lo when the range is empty.
func between(lo, hi time.Duration) time.Duration {
	if hi <= lo {
		return lo
	}
	return lo + rand.N(hi-lo)
}

// sleep waits for d or until ctx is done. It reports whether the full duration elapsed.
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func isNoAction(err error) bool {
	var e interface{ NoAction() bool }
	return errors.As(err, &e) && e.NoAction()
}

func isConflict(err error) bool {
	var e interface{ Conflict() bool }
	return errors.As(err, &e) && e.Conflict()
}

func isPermanent(err error) bool {
	var e interface{ Permanent() bool }
	return errors.As(err, &e) && e.Permanent()
}
