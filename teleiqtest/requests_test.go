package teleiqtest_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
	"github.com/DhurghamAhmed/teleiq/teleiqtest"
)

// callGetMe calls getMe on a connection of its own, reads the answer and leaves the connection
// open.
func callGetMe(t *testing.T, srv *teleiqtest.Server) *net.TCPConn {
	t.Helper()
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(conn, "POST /bot%s/getMe HTTP/1.1\r\nHost: teleiqtest\r\nContent-Length: 0\r\n\r\n", teleiqtest.Token); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("getMe = %v, %v", resp, err)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	}
	return conn.(*net.TCPConn)
}

// TestStopAfterWait stops a bot as soon as Wait returns, which must not cut its call short.
func TestStopAfterWait(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	for i := range 200 {
		stop := teleiqtest.Start(t, func(ctx context.Context) error {
			c, err := teleiq.NewClient(teleiqtest.Token, teleiq.WithBaseURL(srv.URL))
			if err != nil {
				return err
			}
			commands := []models.BotCommand{{Command: "start", Description: "Start"}}
			if err := c.SetMyCommands(ctx, teleiq.SetMyCommandsParams{Commands: commands}); err != nil {
				return err
			}
			<-ctx.Done()
			return nil
		})
		srv.Wait(t, "setMyCommands", i+1)
		if err := stop(); err != nil {
			t.Fatalf("stop() right after Wait = %v, want nil", err)
		}
	}
}

// TestWaitForTheAnswer checks that Requests shows a request at once, while Wait waits until the
// bot has read the answer, or a while when it keeps the connection open, as if it had not.
func TestWaitForTheAnswer(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	start := time.Now()
	callGetMe(t, srv)
	if got := len(srv.Requests("getMe")); got != 1 {
		t.Errorf("%d getMe requests recorded once answered, want 1", got)
	}
	srv.Wait(t, "getMe", 1)
	if waited := time.Since(start); waited < 500*time.Millisecond {
		t.Errorf("Wait returned after %v, before the answer could be taken as read", waited)
	}
}

// TestResetAfterTheBotCloses checks that the server resets a connection that the bot closed: a
// clean close would hold a port of the bot in TIME_WAIT, and a test that makes many calls would
// run out of ports.
func TestResetAfterTheBotCloses(t *testing.T) {
	srv := teleiqtest.NewServer()
	defer srv.Close()
	conn := callGetMe(t, srv)
	if err := conn.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Read(make([]byte, 1)); err == nil || errors.Is(err, io.EOF) {
		t.Errorf("reading after the bot closed = %v, want the connection reset", err)
	}
}
