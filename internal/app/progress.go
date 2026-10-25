package app

import (
	"context"
	"time"
)

// Progress is reported while a job runs.
type Progress struct {
	Stage    string // "scan", "identify", "generate", "convert", "verify", "dat", "chd"
	SystemID string
	Profile  string
	Detail   string
	// Item is the thing being worked on (a game, a file), when there is one.
	Item string
	// Done and Total count the steps of the whole job (0 = not known).
	Done, Total int
	// Percent is how far a running conversion is (Stage "convert"), 0-100.
	Percent float64
}

// reporter sends a job's progress to the caller's callback (which may be nil).
// A job is a known number of steps; step announces the next one, item names
// what is being worked on within it (a file, a game, a conversion's percent),
// and next marks the step finished. Every Progress carries the step counts.
type reporter struct {
	ctx         context.Context
	send        func(Progress)
	delay       time.Duration // test hook: pause after each step
	done, total int
}

func (a *App) reporter(ctx context.Context, send func(Progress), total int) *reporter {
	return &reporter{ctx: ctx, send: send, delay: a.stepDelay, total: total}
}

// step announces a step. Under the ROMCURE_STEP_DELAY_MS test hook it then
// pauses, so short jobs last long enough to watch and cancel.
func (r *reporter) step(p Progress) {
	r.item(p)
	if r.delay > 0 {
		select {
		case <-time.After(r.delay):
		case <-r.ctx.Done():
		}
	}
}

// item reports progress within the current step. These can come thick and
// fast (one per file); the receiver thins them out.
func (r *reporter) item(p Progress) {
	if r.send != nil {
		p.Done, p.Total = r.done, r.total
		r.send(p)
	}
}

// next marks the current step finished.
func (r *reporter) next() { r.done++ }
