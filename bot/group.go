package bot

import "errors"

// Group is a set of handlers that share a filter and middleware.
type Group struct {
	bot        *Bot
	filter     Filter // nil matches every update
	routes     []route
	middleware []Middleware
}

// Group returns a group whose handlers run only when every filter matches.
func (b *Bot) Group(filters ...Filter) *Group { return b.group(&b.routes, "Group", filters) }

// Group returns a nested group whose handlers run only when every filter matches.
func (g *Group) Group(filters ...Filter) *Group { return g.bot.group(&g.routes, "Group", filters) }

func (b *Bot) group(routes *[]route, method string, filters []Filter) *Group {
	g := &Group{bot: b}
	for _, f := range filters {
		if f == nil {
			b.mu.Lock()
			b.misuse(errors.New("bot: " + method + ": nil filter"))
			b.mu.Unlock()
			return g
		}
	}
	switch len(filters) {
	case 0:
	case 1:
		g.filter = filters[0]
	default:
		g.filter = FilterFunc(func(c *Context) bool {
			for _, f := range filters {
				if !f.Match(c) {
					return false
				}
			}
			return true
		})
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	*routes = append(*routes, route{group: g})
	return g
}

// Use adds middleware around the handlers of g and of the groups inside it.
func (g *Group) Use(mw ...Middleware) {
	b := g.bot
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, m := range mw {
		if m == nil {
			b.misuse(errors.New("bot: Group.Use: nil middleware"))
			return
		}
	}
	g.middleware = append(g.middleware, mw...)
}

// Handle runs h for the updates that the filters of g and f match.
func (g *Group) Handle(f Filter, h Handler) { g.bot.add(&g.routes, f, h) }

// OnMessage runs h for new messages that the filters of g match.
func (g *Group) OnMessage(h Handler) { g.Handle(onMessage, h) }

// OnCallback runs h for callback queries that the filters of g match.
func (g *Group) OnCallback(h Handler) { g.Handle(onCallback, h) }

// OnCommand runs h for the command name in updates that the filters of g match.
func (g *Group) OnCommand(name string, h Handler) { g.bot.addCommand(&g.routes, name, h) }

// OnCallbackPrefix runs h for callback queries in g whose data starts with prefix.
func (g *Group) OnCallbackPrefix(prefix string, h Handler) { g.Handle(callbackPrefix(prefix), h) }

// OnInlineQuery runs h for inline queries that the filters of g match.
func (g *Group) OnInlineQuery(h Handler) { g.Handle(onInlineQuery, h) }

// OnJoinRequest runs h for requests to join a chat that the filters of g match.
func (g *Group) OnJoinRequest(h Handler) { g.Handle(onJoinRequest, h) }
