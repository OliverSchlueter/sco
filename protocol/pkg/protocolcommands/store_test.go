package protocolcommands

import (
	"bytes"
	"errors"
	"testing"

	"github.com/OliverSchlueter/sco-protocol/pkg/protocol"
)

func TestStoreExecuteReturnsCommandNotFound(t *testing.T) {
	store := New()
	cmd := &protocol.Command{ReqID: 7, ID: 42, Payload: []byte("ping")}

	resp := store.Execute(cmd)
	if resp == nil {
		t.Fatal("Execute() returned nil response")
	}
	if resp.Code != protocol.StatusCommandNotFound {
		t.Fatalf("Execute() code = %d, want %d", resp.Code, protocol.StatusCommandNotFound)
	}
	if got, want := string(resp.Payload), "command with ID 42 not found"; got != want {
		t.Fatalf("Execute() payload = %q, want %q", got, want)
	}
}

func TestStoreExecuteCallsRegisteredHandler(t *testing.T) {
	store := New()
	cmd := &protocol.Command{ReqID: 11, ID: 9, Payload: []byte("hello")}

	var called *protocol.Command
	err := store.RegisterHandler(cmd.ID, func(in *protocol.Command) (*protocol.Response, error) {
		called = in
		return &protocol.Response{
			ReqID:   in.ReqID,
			Code:    protocol.StatusCodeOK,
			Payload: []byte("ok"),
		}, nil
	})
	if err != nil {
		t.Fatalf("RegisterHandler() unexpected error: %v", err)
	}

	resp := store.Execute(cmd)
	if resp == nil {
		t.Fatal("Execute() returned nil response")
	}
	if called != cmd {
		t.Fatalf("handler called with %p, want %p", called, cmd)
	}
	if resp.Code != protocol.StatusCodeOK {
		t.Fatalf("Execute() code = %d, want %d", resp.Code, protocol.StatusCodeOK)
	}
	if !bytes.Equal(resp.Payload, []byte("ok")) {
		t.Fatalf("Execute() payload = %q, want %q", resp.Payload, []byte("ok"))
	}
}

func TestStoreExecuteAppliesMiddlewareAndHandlerErrors(t *testing.T) {
	store := New()
	var calls []string

	store.RegisterMiddleware(func(next Handler) Handler {
		return func(cmd *protocol.Command) (*protocol.Response, error) {
			calls = append(calls, "mw1-before")
			resp, err := next(cmd)
			calls = append(calls, "mw1-after")
			if err != nil {
				return nil, err
			}
			resp.Payload = append(resp.Payload, []byte("-done")...)
			return resp, nil
		}
	})
	store.RegisterMiddleware(func(next Handler) Handler {
		return func(cmd *protocol.Command) (*protocol.Response, error) {
			calls = append(calls, "mw2-before")
			resp, err := next(cmd)
			calls = append(calls, "mw2-after")
			return resp, err
		}
	})

	if err := store.RegisterHandler(123, func(cmd *protocol.Command) (*protocol.Response, error) {
		calls = append(calls, "handler")
		return &protocol.Response{ReqID: cmd.ReqID, Code: protocol.StatusCodeOK, Payload: []byte("ok")}, nil
	}); err != nil {
		t.Fatalf("RegisterHandler() unexpected error: %v", err)
	}

	resp := store.Execute(&protocol.Command{ReqID: 99, ID: 123, Payload: []byte("ping")})
	if resp == nil {
		t.Fatal("Execute() returned nil response")
	}
	if want := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}; !equalStringSlice(calls, want) {
		t.Fatalf("middleware call order = %v, want %v", calls, want)
	}
	if !bytes.Equal(resp.Payload, []byte("ok-done")) {
		t.Fatalf("Execute() payload = %q, want %q", resp.Payload, []byte("ok-done"))
	}

	if err := store.RegisterHandler(456, func(cmd *protocol.Command) (*protocol.Response, error) {
		return nil, errors.New("boom")
	}); err != nil {
		t.Fatalf("RegisterHandler() unexpected error: %v", err)
	}

	resp = store.Execute(&protocol.Command{ReqID: 100, ID: 456, Payload: []byte("fail")})
	if resp == nil {
		t.Fatal("Execute() returned nil response")
	}
	if resp.Code != protocol.StatusInternalError {
		t.Fatalf("Execute() error code = %d, want %d", resp.Code, protocol.StatusInternalError)
	}
	if got, want := string(resp.Payload), "internal server error: boom"; got != want {
		t.Fatalf("Execute() error payload = %q, want %q", got, want)
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
