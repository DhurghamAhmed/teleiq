package bot

import (
	"context"
	"errors"
	"strings"

	"github.com/DhurghamAhmed/teleiq/models"
)

// Handler handles an update; its error goes to the ErrorHandler of the bot.
type Handler func(ctx context.Context, c *Context) error

// ErrorHandler receives the errors of handlers, of receiving updates and of webhooks.
type ErrorHandler func(ctx context.Context, c *Context, err error)

// Filter selects the updates that a handler handles.
type Filter interface {
	Match(c *Context) bool
}

// FilterFunc adapts a function to Filter.
type FilterFunc func(c *Context) bool

// Match returns f(c).
func (f FilterFunc) Match(c *Context) bool { return f(c) }

// route is a handler with its filter, or a group of routes.
type route struct {
	filter  Filter
	handler Handler
	group   *Group
}

// Handle runs h for the updates that f matches.
func (b *Bot) Handle(f Filter, h Handler) { b.add(&b.routes, f, h) }

// OnMessage runs h for new messages, the updates with a message field.
func (b *Bot) OnMessage(h Handler) { b.Handle(onMessage, h) }

// OnCallback runs h for callback queries, sent when a user presses an inline keyboard button.
func (b *Bot) OnCallback(h Handler) { b.Handle(onCallback, h) }

// OnCommand runs h for new messages that start with the command name.
func (b *Bot) OnCommand(name string, h Handler) { b.addCommand(&b.routes, name, h) }

// add adds a route to routes, the routes of the bot or of a group, or records the misuse.
func (b *Bot) add(routes *[]route, f Filter, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case f == nil:
		b.misuse(errors.New("bot: Handle: nil filter"))
	case h == nil:
		b.misuse(errors.New("bot: Handle: nil handler"))
	default:
		*routes = append(*routes, route{filter: f, handler: h})
	}
}

func (b *Bot) addCommand(routes *[]route, name string, h Handler) {
	name = strings.TrimPrefix(name, "/")
	if !validCommand(name) {
		b.mu.Lock()
		b.misuse(errors.New("bot: OnCommand: a command has 1 to 32 letters, digits or underscores"))
		b.mu.Unlock()
		return
	}
	b.add(routes, FilterFunc(func(c *Context) bool { return strings.EqualFold(c.Command(), name) }), h)
}

var (
	onMessage     = FilterFunc(func(c *Context) bool { return c.update.Message != nil })
	onCallback    = FilterFunc(func(c *Context) bool { return c.update.CallbackQuery != nil })
	onInlineQuery = FilterFunc(func(c *Context) bool { return c.update.InlineQuery != nil })
	onJoinRequest = FilterFunc(func(c *Context) bool { return c.update.ChatJoinRequest != nil })
)

func callbackPrefix(prefix string) Filter {
	return FilterFunc(func(c *Context) bool {
		q := c.update.CallbackQuery
		return q != nil && q.Data != nil && strings.HasPrefix(*q.Data, prefix)
	})
}

// misuse records the first misuse, which Run returns; b.mu must be held.
func (b *Bot) misuse(err error) {
	if b.err == nil {
		b.err = err
	}
}

// dispatch runs the handler of the first route that matches u, inside the middleware.
func (b *Bot) dispatch(ctx context.Context, u *models.Update) {
	c := b.NewContext(u)
	b.mu.RLock()
	routes, middleware := b.routes, b.middleware
	b.mu.RUnlock()
	h := Handler(func(ctx context.Context, c *Context) error {
		for _, r := range routes {
			next := r.handler
			if r.group != nil {
				next = c.bot.matchGroup(c, r.group) // c.bot, not b: cheaper to capture only routes
			} else if !r.filter.Match(c) {
				next = nil
			}
			if next != nil {
				c.handled = true
				return next(ctx, c)
			}
		}
		return nil
	})
	for i := len(middleware) - 1; i >= 0; i-- {
		if h = middleware[i](h); h == nil {
			b.cfg.errorHandler(ctx, c, errors.New("bot: a middleware returned a nil handler"))
			return
		}
	}
	if err := h(ctx, c); err != nil {
		b.cfg.errorHandler(ctx, c, err)
	}
}

// matchGroup returns the handler of the first route of g that matches c, or nil.
func (b *Bot) matchGroup(c *Context, g *Group) Handler {
	if g.filter != nil && !g.filter.Match(c) {
		return nil
	}
	// Handlers may be added while the bot runs, so the slices are read under the lock.
	b.mu.RLock()
	routes, middleware := g.routes, g.middleware
	b.mu.RUnlock()
	for _, r := range routes {
		next := r.handler
		if r.group != nil {
			next = b.matchGroup(c, r.group)
		} else if !r.filter.Match(c) {
			next = nil
		}
		if next != nil {
			return wrap(next, middleware)
		}
	}
	return nil
}

// wrap puts h inside middleware, the first one outermost.
func wrap(h Handler, middleware []Middleware) Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		if h = middleware[i](h); h == nil {
			return func(context.Context, *Context) error { return errors.New("bot: a middleware returned a nil handler") }
		}
	}
	return h
}
