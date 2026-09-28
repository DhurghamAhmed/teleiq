package teleiqtest

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DhurghamAhmed/teleiq"
	"github.com/DhurghamAhmed/teleiq/models"
)

// Token is a well-formed bot token for teleiq.NewClient. The server accepts any token.
const Token = "123456:TEST"

// waitTimeout is how long Wait and WaitHandled wait.
const waitTimeout = 5 * time.Second

// readTimeout is how long the server waits for the bot to read an answer.
const readTimeout = time.Second

// Server is a fake Telegram Bot API server. Its methods are safe for concurrent use.
type Server struct {
	// URL is the base URL of the server, for teleiq.WithBaseURL.
	URL string

	http *httptest.Server

	mu          sync.Mutex
	changed     chan struct{} // closed and replaced when anything changes, to wake whoever waits
	closed      chan struct{}
	lastUpdate  int64
	lastMessage int64
	lastFile    int64
	updates     []models.Update // not yet confirmed, in the order of their update_id
	offset      int64           // the highest offset the bot has sent
	requests    []recorded      // in the order of their answers
	stubs       map[string]stub
	files       map[string]*storedFile // by file_id
	paths       map[string]*storedFile // by file_path
}

// recorded is an answered request and whether the bot has read the answer.
type recorded struct {
	Request
	read bool
}

// connKey is the context key of the connection that carries a request.
type connKey struct{}

// stub is the answer that Respond or Fail set for a method.
type stub struct {
	result json.RawMessage
	err    *teleiq.Error
}

// NewServer starts a Server. Call Close when the test is done.
func NewServer() *Server {
	s := &Server{
		changed: make(chan struct{}),
		closed:  make(chan struct{}),
		stubs:   map[string]stub{},
		files:   map[string]*storedFile{},
		paths:   map[string]*storedFile{},
	}
	s.http = httptest.NewUnstartedServer(http.HandlerFunc(s.serve))
	s.http.Config.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
		return context.WithValue(ctx, connKey{}, c)
	}
	s.http.Start()
	s.URL = s.http.URL
	return s
}

// Close shuts the server down. Long polls in progress end at once.
func (s *Server) Close() {
	s.mu.Lock()
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	s.mu.Unlock()
	s.http.Close()
}

// Respond makes later calls to method answer with result until Reset.
func (s *Server) Respond(method string, result any) {
	data, err := json.Marshal(result)
	if err != nil {
		panic("teleiqtest: Respond: " + err.Error())
	}
	s.mu.Lock()
	s.stubs[method] = stub{result: data}
	s.mu.Unlock()
}

// Fail makes later calls to method fail with err until Reset.
func (s *Server) Fail(method string, err *teleiq.Error) {
	if err == nil {
		panic("teleiqtest: Fail: nil error")
	}
	s.mu.Lock()
	s.stubs[method] = stub{err: err}
	s.mu.Unlock()
}

// Reset makes method give its default answer again, undoing Respond and Fail.
func (s *Server) Reset(method string) {
	s.mu.Lock()
	delete(s.stubs, method)
	s.mu.Unlock()
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if rest, ok := strings.CutPrefix(r.URL.Path, "/file/bot"); ok {
		s.download(w, rest)
		return
	}
	rest, isBot := strings.CutPrefix(r.URL.Path, "/bot")
	_, method, ok := strings.Cut(rest, "/")
	if !isBot || !ok || method == "" {
		writeError(w, &teleiq.Error{ErrorCode: http.StatusNotFound, Description: "Not Found"})
		return
	}
	req, err := readRequest(method, r)
	// Drain the body so the server notices when the bot closes the connection.
	_, _ = io.Copy(io.Discard, r.Body)
	if err != nil {
		writeError(w, &teleiq.Error{ErrorCode: http.StatusBadRequest, Description: "Bad Request: " + err.Error()})
		return
	}
	if method == "getUpdates" {
		s.getUpdates(w, r, req)
		return
	}
	s.mu.Lock()
	result, apiErr := s.answer(req)
	req.Result = result
	i := len(s.requests)
	s.requests = append(s.requests, recorded{Request: req})
	s.notify()
	s.mu.Unlock()
	w.Header().Set("Connection", "close")
	if apiErr != nil {
		writeError(w, apiErr)
	} else {
		writeResult(w, result)
	}
	s.awaitRead(w, r)
	s.mu.Lock()
	s.requests[i].read = true
	s.notify()
	s.mu.Unlock()
}

// awaitRead sends the answer to r and waits until the bot has read it.
func (s *Server) awaitRead(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).Flush()
	timer := time.NewTimer(readTimeout)
	defer timer.Stop()
	select {
	case <-r.Context().Done():
		// A reset avoids TIME_WAIT, which would exhaust ports in tests with many calls.
		if c, ok := r.Context().Value(connKey{}).(*net.TCPConn); ok {
			_ = c.SetLinger(0)
		}
	case <-s.closed:
	case <-timer.C:
	}
}

// answer returns the result or the error for a request; s.mu must be held.
func (s *Server) answer(req Request) (json.RawMessage, *teleiq.Error) {
	if stub, ok := s.stubs[req.Method]; ok {
		return stub.result, stub.err
	}
	v, apiErr := s.defaultResult(req)
	if apiErr != nil {
		return nil, apiErr
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, &teleiq.Error{ErrorCode: http.StatusInternalServerError, Description: "teleiqtest: " + err.Error()}
	}
	return data, nil
}

// notify wakes whoever waits for a change; s.mu must be held.
func (s *Server) notify() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// waitFor waits up to waitTimeout for cond, checked with s.mu held, or fails t.
func (s *Server) waitFor(t testing.TB, what string, cond func() bool) {
	t.Helper()
	timer := time.NewTimer(waitTimeout)
	defer timer.Stop()
	for {
		s.mu.Lock()
		done, changed := cond(), s.changed
		s.mu.Unlock()
		if done {
			return
		}
		select {
		case <-changed:
		case <-s.closed:
			t.Fatalf("teleiqtest: the server was closed while waiting for %s", what)
		case <-timer.C:
			t.Fatalf("teleiqtest: timed out waiting for %s", what)
		}
	}
}

func writeResult(w http.ResponseWriter, result any) {
	body, err := json.Marshal(struct {
		OK     bool `json:"ok"`
		Result any  `json:"result"`
	}{true, result})
	if err != nil {
		writeError(w, &teleiq.Error{ErrorCode: http.StatusInternalServerError, Description: "teleiqtest: " + err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}

func writeError(w http.ResponseWriter, e *teleiq.Error) {
	code := e.ErrorCode
	if code == 0 {
		code = http.StatusBadRequest
	}
	body, _ := json.Marshal(struct {
		OK          bool                       `json:"ok"`
		ErrorCode   int                        `json:"error_code"`
		Description string                     `json:"description"`
		Parameters  *models.ResponseParameters `json:"parameters,omitempty"`
	}{false, code, e.Description, e.Parameters})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(code)
	_, _ = w.Write(body)
}
