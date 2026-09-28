package bot

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// ErrShutdownTimeout reports updates still in progress at the shutdown timeout.
var ErrShutdownTimeout = errors.New("bot: shutdown timed out before the updates in progress were handled")

// pollConfig holds the settings of the poller.
type pollConfig struct {
	timeout         time.Duration // how long getUpdates waits for updates
	limit           int           // the most updates in one batch
	allowedUpdates  []string
	shutdownTimeout time.Duration // how long a shutdown waits for the updates in progress
	ackTimeout      time.Duration // how long the final confirmation may take
	retryBase       time.Duration // the delay after a failed getUpdates, doubled up to retryMax
	retryMax        time.Duration
	onError         func(error) // receives the failures of getUpdates that are retried
}

const (
	// pollMargin extends the deadline of getUpdates beyond its long polling timeout.
	pollMargin = 10 * time.Second

	// idleResync is the idle time after which the poller stops sending its offset.
	idleResync = 6 * 24 * time.Hour
)

// refetchInterval is the wait before asking again while an update is open.
const refetchInterval = 500 * time.Millisecond

// poller receives updates with getUpdates and hands each new one to the scheduler.
type poller struct {
	client *teleiq.Client
	sched  *scheduler
	cfg    pollConfig
	track  *tracker
	done   func(*models.Update) // track.handled, bound once so that handing over does not allocate

	sent       int64 // the last offset Telegram received; 0 before any
	lastUpdate time.Time
	now        func() time.Time
}

func newPoller(client *teleiq.Client, s *scheduler, cfg pollConfig) *poller {
	p := &poller{client: client, sched: s, cfg: cfg, track: newTracker(), now: time.Now}
	p.done = p.track.handled
	return p
}

// wallNow returns the time without its monotonic reading, so suspends are counted.
func (p *poller) wallNow() time.Time {
	return p.now().Round(0)
}

// idle reports whether no update has arrived for longer than idleResync.
func (p *poller) idle() bool {
	return p.wallNow().Sub(p.lastUpdate) > idleResync
}

// run polls until ctx ends or getUpdates fails for good, then drains and confirms.
func (p *poller) run(ctx context.Context, handle func(context.Context, *models.Update)) error {
	// Handlers outlive ctx during the drain; the shutdown timeout cancels them.
	handlerCtx, cancelHandlers := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelHandlers()
	p.sched.start(handlerCtx, handle)

	var fatal error
	news, full := true, false
	for delay := time.Duration(0); ctx.Err() == nil; {
		if !news {
			p.pause(ctx, full)
			if ctx.Err() != nil {
				break
			}
		}
		updates, err := p.fetch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			if isFatal(err) {
				fatal = err
				break
			}
			if p.cfg.onError != nil {
				p.cfg.onError(err)
			}
			delay = min(max(2*delay, p.cfg.retryBase), p.cfg.retryMax)
			sleep(ctx, delay/2+rand.N(delay/2+1))
			continue
		}
		delay = 0
		handed, ok := p.handOver(ctx, updates)
		if !ok {
			break
		}
		news, full = handed > 0, len(updates) >= p.cfg.limit
	}

	p.sched.close()
	drain, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.cfg.shutdownTimeout)
	defer cancel()
	if err := p.sched.wait(drain); err != nil {
		// Freeze first so handlers canceled at the timeout do not count as handled.
		p.track.freeze()
		cancelHandlers()
		p.confirm(ctx)
		return ErrShutdownTimeout
	}
	p.confirm(ctx)
	return fatal
}

// pause waits before asking again while an update is open.
func (p *poller) pause(ctx context.Context, full bool) {
	var tick <-chan time.Time
	if !full {
		t := time.NewTimer(refetchInterval)
		defer t.Stop()
		tick = t.C
	}
	for {
		if _, open := p.track.offset(); !open {
			return
		}
		select {
		case <-p.track.moved:
			if full {
				return
			}
		case <-tick:
			return
		case <-ctx.Done():
			return
		}
	}
}

// fetch asks getUpdates for the updates from the offset on.
func (p *poller) fetch(ctx context.Context) ([]models.Update, error) {
	ctx, cancel := context.WithTimeout(ctx, p.cfg.timeout+pollMargin)
	defer cancel()
	offset, open := p.track.offset()
	params := teleiq.GetUpdatesParams{
		Limit:          &p.cfg.limit,
		Timeout:        teleiq.Ptr(int(p.cfg.timeout / time.Second)),
		AllowedUpdates: p.cfg.allowedUpdates,
	}
	// Omitting the offset rebuilds it from the next update; never while one is open.
	resync := offset != 0 && !open && p.idle()
	if offset != 0 && !resync {
		params.Offset = &offset
	}
	updates, err := p.client.GetUpdates(ctx, params)
	if err != nil {
		return nil, err
	}
	switch {
	case resync:
		p.track.resync()
	case params.Offset != nil:
		p.sent = offset
	}
	if len(updates) > 0 {
		p.lastUpdate = p.wallNow()
	}
	return updates, nil
}

// handOver hands the unseen updates to the scheduler and reports how many.
func (p *poller) handOver(ctx context.Context, updates []models.Update) (int, bool) {
	n := 0
	for i := range updates {
		u := &updates[i]
		if !p.track.add(u.UpdateID) {
			continue
		}
		if err := p.sched.submit(ctx, job{update: u, done: p.done}); err != nil {
			return n, false
		}
		n++
	}
	return n, true
}

// confirm tells Telegram the final offset with one last getUpdates.
func (p *poller) confirm(ctx context.Context) {
	offset, _ := p.track.offset()
	if offset == 0 || offset == p.sent {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.cfg.ackTimeout)
	defer cancel()
	_, _ = p.client.GetUpdates(ctx, teleiq.GetUpdatesParams{Offset: &offset, Limit: teleiq.Ptr(1), Timeout: teleiq.Ptr(0)})
}

// isFatal reports failures of getUpdates that retrying cannot fix.
func isFatal(err error) bool {
	var apiErr *teleiq.Error
	return errors.As(err, &apiErr) && apiErr.ErrorCode >= 400 && apiErr.ErrorCode < 500 &&
		apiErr.ErrorCode != http.StatusTooManyRequests
}

// sleep waits for d, returning early when ctx ends.
func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
