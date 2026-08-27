package protocolcommandstore

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/OliverSchlueter/sco-protocol/pkg/protocol"
)

func TestStoreExecuteReturnsCommandNotFound(t *testing.T) {
	store := New()
	ctx := &ConnCtx{ID: "conn-1"}
	msg := &protocol.Message{Type: byte(protocol.MessageTypeCommand)}
	cmd := &protocol.Command{ReqID: 7, ID: 42, Payload: []byte("ping")}

	resp := store.Execute(ctx, msg, cmd)
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
	ctx := &ConnCtx{ID: "conn-2"}
	msg := &protocol.Message{Type: byte(protocol.MessageTypeCommand)}
	cmd := &protocol.Command{ReqID: 11, ID: 9, Payload: []byte("hello")}

	var calledCtx *ConnCtx
	var calledMsg *protocol.Message
	var calledCmd *protocol.Command
	if err := store.RegisterHandler(cmd.ID, func(inCtx *ConnCtx, inMsg *protocol.Message, inCmd *protocol.Command) (*protocol.Response, error) {
		calledCtx = inCtx
		calledMsg = inMsg
		calledCmd = inCmd
		return &protocol.Response{
			ReqID:   inCmd.ReqID,
			Code:    protocol.StatusCodeOK,
			Payload: []byte("ok"),
		}, nil
	}); err != nil {
		t.Fatalf("RegisterHandler() unexpected error: %v", err)
	}

	resp := store.Execute(ctx, msg, cmd)
	if resp == nil {
		t.Fatal("Execute() returned nil response")
	}
	if calledCtx != ctx || calledMsg != msg || calledCmd != cmd {
		t.Fatalf("handler called with ctx=%p msg=%p cmd=%p; want ctx=%p msg=%p cmd=%p", calledCtx, calledMsg, calledCmd, ctx, msg, cmd)
	}
	if resp.Code != protocol.StatusCodeOK {
		t.Fatalf("Execute() code = %d, want %d", resp.Code, protocol.StatusCodeOK)
	}
	if !bytes.Equal(resp.Payload, []byte("ok")) {
		t.Fatalf("Execute() payload = %q, want %q", resp.Payload, []byte("ok"))
	}
}

func TestStoreExecuteAppliesMiddlewareInReverseRegistrationOrder(t *testing.T) {
	store := New()
	ctx := &ConnCtx{ID: "conn-3"}
	msg := &protocol.Message{Type: byte(protocol.MessageTypeCommand)}
	var calls []string

	store.RegisterMiddleware(func(next Handler) Handler {
		return func(inCtx *ConnCtx, inMsg *protocol.Message, inCmd *protocol.Command) (*protocol.Response, error) {
			calls = append(calls, "mw1-before")
			resp, err := next(inCtx, inMsg, inCmd)
			calls = append(calls, "mw1-after")
			if err != nil {
				return nil, err
			}
			resp.Payload = append(resp.Payload, []byte("-done")...)
			return resp, nil
		}
	})
	store.RegisterMiddleware(func(next Handler) Handler {
		return func(inCtx *ConnCtx, inMsg *protocol.Message, inCmd *protocol.Command) (*protocol.Response, error) {
			calls = append(calls, "mw2-before")
			resp, err := next(inCtx, inMsg, inCmd)
			calls = append(calls, "mw2-after")
			return resp, err
		}
	})

	if err := store.RegisterHandler(123, func(inCtx *ConnCtx, inMsg *protocol.Message, inCmd *protocol.Command) (*protocol.Response, error) {
		calls = append(calls, "handler")
		return &protocol.Response{ReqID: inCmd.ReqID, Code: protocol.StatusCodeOK, Payload: []byte("ok")}, nil
	}); err != nil {
		t.Fatalf("RegisterHandler() unexpected error: %v", err)
	}

	resp := store.Execute(ctx, msg, &protocol.Command{ReqID: 99, ID: 123, Payload: []byte("ping")})
	if resp == nil {
		t.Fatal("Execute() returned nil response")
	}
	if want := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("middleware call order = %v, want %v", calls, want)
	}
	if !bytes.Equal(resp.Payload, []byte("ok-done")) {
		t.Fatalf("Execute() payload = %q, want %q", resp.Payload, []byte("ok-done"))
	}
}

func TestStoreExecuteReturnsInternalErrorWhenHandlerFails(t *testing.T) {
	store := New()
	ctx := &ConnCtx{ID: "conn-4"}
	msg := &protocol.Message{Type: byte(protocol.MessageTypeCommand)}
	if err := store.RegisterHandler(456, func(inCtx *ConnCtx, inMsg *protocol.Message, inCmd *protocol.Command) (*protocol.Response, error) {
		return nil, errors.New("boom")
	}); err != nil {
		t.Fatalf("RegisterHandler() unexpected error: %v", err)
	}

	resp := store.Execute(ctx, msg, &protocol.Command{ReqID: 100, ID: 456, Payload: []byte("fail")})
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
