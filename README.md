# TeleIQ

[![Go Reference](https://pkg.go.dev/badge/github.com/DhurghamAhmed/teleiq.svg)](https://pkg.go.dev/github.com/DhurghamAhmed/teleiq)
[![Bot API 10.3](https://img.shields.io/badge/Bot%20API-10.3-2CA5E0?logo=telegram)](https://core.telegram.org/bots/api)
[![Go 1.27+](https://img.shields.io/badge/Go-1.27%2B-00ADD8?logo=go)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue)](LICENSE)

[![Telegram channel](https://img.shields.io/badge/Channel-%40teleiqgo-2CA5E0?logo=telegram)](https://t.me/teleiqgo)
[![Telegram group](https://img.shields.io/badge/Group-%40teleiqgroup-2CA5E0?logo=telegram)](https://t.me/teleiqgroup)

<p align="center">
  <img src="assets/logo.png" alt="TeleIQ logo" width="512">
</p>

TeleIQ is a Go library for the [Telegram Bot API](https://core.telegram.org/bots/api),
with a framework for bots that run for months.

Every type and method of Bot API 10.3 is typed Go code, generated from the
official documentation. The framework receives updates by long polling or
webhook, handles the updates of each chat in order, retries failed requests,
and never writes the bot token to errors or logs.

> [!NOTE]
> TeleIQ uses only the Go standard library. Until v1.0.0, a minor release may
> change the API.

### Table of contents

<details>
<summary>Show or hide</summary>

- [Getting started](#getting-started)
  - [Basic setup](#basic-setup)
  - [Getting updates](#getting-updates)
  - [Calling methods](#calling-methods)
  - [Helpers](#helpers)
  - [Client options](#client-options)
  - [Bot handlers](#bot-handlers)
- [License](#license)

</details>

## Getting started

```bash
go get github.com/DhurghamAhmed/teleiq@latest
```

TeleIQ needs Go 1.27 or later and a bot token from
[@BotFather](https://t.me/BotFather).

<details>
<summary>Examples</summary>

Each example is a complete program with a test. Run one with
`BOT_TOKEN='<token>' go run ./examples/<name>`.

| Example | Shows |
| --- | --- |
| [`hello`](examples/hello) | The bot of Basic setup: greets `/start` and repeats messages |
| [`basic`](examples/basic) | A token check with `getMe`, and a message to `CHAT_ID` |
| [`commands`](examples/commands) | `/start`, `/help` and `/echo` in the command menu |
| [`handlers`](examples/handlers) | Every way to add a handler, and the order in which they match |
| [`filters`](examples/filters) | Filters combined with `And`, `Or` and `Not` |
| [`callback`](examples/callback) | Inline buttons and the answer to a press |
| [`shop`](examples/shop) | Buttons that carry typed data, laid out in a grid |
| [`moderation`](examples/moderation) | `/ban`, `/mute` and a tidy group |
| [`inline`](examples/inline) | Inline mode |
| [`middleware`](examples/middleware) | Recovery, logging and a middleware of its own |
| [`fsm`](examples/fsm) | A conversation of two questions |
| [`files`](examples/files) | An upload to `CHAT_ID` and its download; `FILE` picks the file |
| [`webhook`](examples/webhook) | Updates by webhook; needs `WEBHOOK_URL` and `WEBHOOK_SECRET` |
| [`webapp`](examples/webapp) | A Mini App whose data the server checks; needs `WEBAPP_URL` |
| [`graceful-shutdown`](examples/graceful-shutdown) | A slow handler that finishes on Ctrl+C |

</details>

<details>
<summary>Useful resources</summary>

- [Telegram Bot API](https://core.telegram.org/bots/api) and its
  [changelog](https://core.telegram.org/bots/api-changelog)
- [TeleIQ on pkg.go.dev](https://pkg.go.dev/github.com/DhurghamAhmed/teleiq)
- [Webhooks](https://core.telegram.org/bots/webhooks)
- [Mini Apps](https://core.telegram.org/bots/webapps)
- [Local Bot API Server](https://github.com/tdlib/telegram-bot-api)

</details>

### Basic setup

A complete bot that greets `/start` and repeats every other message:

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/bot"
)

func main() {
	// ctx ends on Ctrl+C, which stops the bot.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client, err := teleiq.NewClient(os.Getenv("BOT_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}
	b := bot.New(client) // b is the bot
	b.OnCommand("start", bot.ReplyWith("Hello! Send me a message and I will repeat it."))
	// c is the current message; c.Send replies in its chat.
	b.OnMessage(func(ctx context.Context, c *bot.Context) error {
		return c.Send(ctx, "You said: "+c.Text())
	})
	if err := b.Run(ctx); err != nil { // receives updates until ctx ends
		log.Fatal(err)
	}
}
```

```bash
export BOT_TOKEN='<token from @BotFather>'
go run .
```

### Getting updates

`Run` receives updates by long polling until its context ends. For a
webhook, serve `WebhookHandler`, register its URL, and call `RunWebhook`:

```go
b := bot.New(client, bot.WithSecretToken(secret))
srv := &http.Server{Addr: ":8080", Handler: b.WebhookHandler(), ReadHeaderTimeout: 10 * time.Second}
go srv.ListenAndServe()

err := client.SetWebhook(ctx, teleiq.SetWebhookParams{
	URL:            "https://bot.example.com/telegram",
	SecretToken:    teleiq.Ptr(secret),
	AllowedUpdates: []string{}, // the default kinds of updates
})
if err == nil {
	err = b.RunWebhook(ctx) // until ctx ends
}
```

- The updates of a chat are handled one at a time and in order; up to
  `WithWorkers` chats (8 by default) are handled at once.
- An update is confirmed to Telegram only once it is handled, so none is lost
  when the bot stops.
- When its context ends, the bot waits up to `WithShutdownTimeout` (30s by
  default) for the handlers in progress.

### Calling methods

`teleiq.Client` has a method for every Bot API method, and package `models`
holds the types:

```go
client, err := teleiq.NewClient(os.Getenv("BOT_TOKEN"))
if err != nil {
	log.Fatal(err)
}
me, err := client.GetMe(ctx)
if err != nil {
	log.Fatal(err)
}
_, err = client.SendMessage(ctx, teleiq.SendMessageParams{
	ChatID:              models.ID(123456789), // or models.Username("@channel")
	Text:                "Hello from " + me.FirstName,
	DisableNotification: teleiq.Ptr(true),
})
switch {
case errors.Is(err, teleiq.ErrBotBlocked):
	// The user blocked the bot.
case errors.Is(err, teleiq.ErrTooManyRequests):
	// Flood control that the retries could not wait out.
}
```

- Required parameters are plain fields; optional ones are pointers, set with
  `teleiq.Ptr`.
- Errors match by name with `errors.Is`, such as `ErrBotBlocked`,
  `ErrChatNotFound`, `ErrMessageNotModified` and `ErrNotEnoughRights`.
- `client.Call` reaches a method that the Bot API added after this release.

### Helpers

| Need | Helpers |
| --- | --- |
| A chat | `models.ID`, `models.Username` |
| A file to send | `models.FileID`, `FileURL`, `FileFromPath`, `FileFromBytes`, `FileFromReader` |
| A file to receive | `client.GetFile`, then `client.Download` to any `io.Writer` |
| Buttons | `models.NewInlineKeyboard`, `NewInlineRow`, `NewCallbackButton`, `NewURLButton`, `NewInlineGrid` |
| Formatting | `teleiq.ParseModeHTML`, `ParseModeMarkdown`, `EscapeMarkdown` |
| Optional values | `teleiq.Ptr` |

Uploads and downloads are streamed, without holding whole files in memory.

### Client options

The options of `teleiq.NewClient` apply to every request:

```go
client, err := teleiq.NewClient(os.Getenv("BOT_TOKEN"),
	teleiq.WithDefaults(teleiq.Defaults{ParseMode: teleiq.ParseModeHTML}),
	teleiq.WithRateLimiter(teleiq.NewRateLimiter(30, time.Second, 3*time.Second)),
	teleiq.WithLogger(slog.Default()),
)
```

| Option | Sets |
| --- | --- |
| `WithDefaults` | The parse mode, link previews, silent or protected messages of calls that leave them unset |
| `WithRetryPolicy` | Retries after network errors, 5xx and flood control; 3 attempts by default |
| `WithRateLimiter` | The pace of requests, in all and per chat |
| `WithDefaultTimeout` | The timeout of calls without a deadline; 30s by default |
| `WithLogger` | A `*slog.Logger` for requests and retries |
| `WithBaseURL` | Another server, such as a Local Bot API Server |
| `WithHTTPClient`, `WithTransport` | How requests reach the server |

### Bot handlers

Package `bot` gives each update to the first handler, in the order they were
added, that matches it:

```go
b := bot.New(client, bot.WithCommands(
	models.BotCommand{Command: "menu", Description: "Pick a color"},
))
b.Use(bot.Recovery(), bot.Logging()) // middleware around every update

b.OnCommand("menu", func(ctx context.Context, c *bot.Context) error {
	return c.Send(ctx, "Pick a color:", bot.Keyboard(models.NewInlineKeyboard(models.NewInlineRow(
		models.NewCallbackButton("Red", "color:red"),
		models.NewCallbackButton("Blue", "color:blue"),
	))))
})
b.OnCallbackPrefix("color:", func(ctx context.Context, c *bot.Context) error {
	color := strings.TrimPrefix(c.CallbackData(), "color:")
	return errors.Join(c.Answer(ctx, "You picked "+color), c.Edit(ctx, "Your color: "+color))
})
b.Handle(filter.And(filter.Group(), filter.Photo()), func(ctx context.Context, c *bot.Context) error {
	return c.Delete(ctx) // no photos in groups
})
```

A group shares one filter and its own middleware among its handlers:

```go
admins := b.Group(isAdmin)
admins.Use(audit)
admins.OnCommand("ban", ban)
b.OnCommand("ban", bot.ReplyWith("Only administrators can ban."))
```

- **Handlers:** `OnCommand`, `OnMessage`, `OnCallback`, `OnCallbackPrefix`,
  `OnInlineQuery`, `OnJoinRequest`, and `Handle` for any filter.
- **Filters** in package `filter`: commands and texts, every kind of media,
  service messages, replies and forwards, chats and users, combined with
  `And`, `Or` and `Not`.
- **Shortcuts** of `bot.Context`: `Send`, `Reply`, `Edit`, `Answer`,
  `Delete`, `Forward`, `Copy`, `Pin`, `React`, `Download`, `Ban`, `Restrict`,
  `Approve`, and every send method of the Bot API, such as `SendPhoto`.
- **More:** typed button data with `bot.NewCallback`, conversations with
  package `fsm`, inline mode with `AnswerInline`, and Mini App data checked
  with package `webapp`.
- **Errors:** a handler's error goes to the `ErrorHandler`, and
  `bot.Recovery()` turns a panic into one.

## License

TeleIQ is released under the [MIT License](LICENSE).
