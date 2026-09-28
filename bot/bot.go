package bot

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// Bot receives updates and runs the handlers that match them.
type Bot struct {
	client *teleiq.Client
	cfg    config

	mu         sync.RWMutex
	routes     []route
	middleware []Middleware
	err        error // the first misuse of New or of adding handlers, which Run returns
	running    atomic.Bool
	me         *models.User            // the bot as getMe describes it, known once Run has started
	hook       atomic.Pointer[webhook] // set while RunWebhook runs
}

// Option configures a Bot created by New.
type Option func(*config) error

type config struct {
	workers      int
	queueSize    int
	errorHandler ErrorHandler
	poll         pollConfig
	secret       []byte // the SHA-256 of the webhook secret token, if set
	maxBody      int64
	async        bool
	commands     []models.BotCommand // set with setMyCommands as the bot starts, if any
}

// New returns a Bot that uses client; Run reports any misuse.
func New(client *teleiq.Client, opts ...Option) *Bot {
	b := &Bot{client: client, cfg: config{
		workers:      8,
		queueSize:    128,
		errorHandler: newDefaultErrorHandler(time.Now),
		maxBody:      1 << 20,
		poll: pollConfig{
			timeout: 30 * time.Second,
			limit:   100,
			// Empty, not nil, so Telegram uses its default, not a list another program set.
			allowedUpdates:  []string{},
			shutdownTimeout: 30 * time.Second,
			ackTimeout:      5 * time.Second,
			retryBase:       time.Second,
			retryMax:        30 * time.Second,
		},
	}}
	if client == nil {
		b.err = errors.New("bot: New: nil client")
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&b.cfg); err != nil && b.err == nil {
			b.err = err
		}
	}
	return b
}

// WithWorkers sets how many updates are handled at once; the default is 8.
func WithWorkers(n int) Option {
	return func(c *config) error {
		if n < 1 {
			return errors.New("bot: WithWorkers: at least one worker is needed")
		}
		c.workers = n
		return nil
	}
}

// WithQueueSize sets how many updates each chat can have waiting; the default is 128.
func WithQueueSize(n int) Option {
	return func(c *config) error {
		if n < 1 {
			return errors.New("bot: WithQueueSize: the queue needs room for at least one update")
		}
		c.queueSize = n
		return nil
	}
}

// WithErrorHandler sets the ErrorHandler; the default logs errors with slog.Default.
func WithErrorHandler(h ErrorHandler) Option {
	return func(c *config) error {
		if h == nil {
			return errors.New("bot: WithErrorHandler: nil handler")
		}
		c.errorHandler = h
		return nil
	}
}

// WithShutdownTimeout sets how long Run waits for running handlers as it stops.
func WithShutdownTimeout(d time.Duration) Option {
	return func(c *config) error {
		if d <= 0 {
			return errors.New("bot: WithShutdownTimeout: the timeout must be positive")
		}
		c.poll.shutdownTimeout = d
		return nil
	}
}

// WithPollTimeout sets how long each getUpdates call waits; the default is 30s.
func WithPollTimeout(d time.Duration) Option {
	return func(c *config) error {
		if d < 0 {
			return errors.New("bot: WithPollTimeout: negative timeout")
		}
		c.poll.timeout = d.Truncate(time.Second)
		return nil
	}
}

// WithAllowedUpdates limits the kinds of updates that Run receives.
func WithAllowedUpdates(kinds ...string) Option {
	return func(c *config) error {
		c.poll.allowedUpdates = append([]string{}, kinds...)
		return nil
	}
}

// Run receives updates with long polling and handles them until ctx ends.
func (b *Bot) Run(ctx context.Context) error {
	if ok, err := b.begin(ctx, "Run"); !ok {
		return err
	}
	defer b.running.Store(false)
	poll := b.cfg.poll
	poll.onError = func(err error) { b.cfg.errorHandler(ctx, nil, err) }
	return newPoller(b.client, newScheduler(b.cfg.workers, b.cfg.queueSize), poll).run(ctx, b.dispatch)
}

// begin marks the bot running and starts it, reporting false when it must not run.
func (b *Bot) begin(ctx context.Context, name string) (bool, error) {
	if ctx == nil {
		return false, errors.New("bot: " + name + ": nil Context")
	}
	b.mu.RLock()
	err := b.err
	b.mu.RUnlock()
	if err != nil {
		return false, err
	}
	if !b.running.CompareAndSwap(false, true) {
		return false, errors.New("bot: " + name + ": the bot is already running")
	}
	if err := b.start(ctx); err != nil {
		b.running.Store(false)
		if ctx.Err() != nil {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// start calls getMe, then setMyCommands if WithCommands was given.
func (b *Bot) start(ctx context.Context) error {
	me, err := b.client.GetMe(ctx)
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.me = me
	b.mu.Unlock()
	if len(b.cfg.commands) == 0 {
		return nil
	}
	return b.client.SetMyCommands(ctx, teleiq.SetMyCommandsParams{Commands: b.cfg.commands})
}
