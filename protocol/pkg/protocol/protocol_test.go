package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"reflect"
	"testing"
	"time"
)

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }

type partialWriteConn struct {
	buf            bytes.Buffer
	remainingShort int
}

func (c *partialWriteConn) Read(p []byte) (int, error) { return 0, io.EOF }
func (c *partialWriteConn) Write(p []byte) (int, error) {
	if c.remainingShort > 0 {
		n := len(p)
		if n > c.remainingShort {
			n = c.remainingShort
		}
		written, err := c.buf.Write(p[:n])
		if err != nil {
			return written, err
		}
		c.remainingShort -= written
		return written, nil
	}
	return c.buf.Write(p)
}
func (*partialWriteConn) Close() error                     { return nil }
func (*partialWriteConn) LocalAddr() net.Addr              { return testAddr("local") }
func (*partialWriteConn) RemoteAddr() net.Addr             { return testAddr("remote") }
func (*partialWriteConn) SetDeadline(time.Time) error      { return nil }
func (*partialWriteConn) SetReadDeadline(time.Time) error  { return nil }
func (*partialWriteConn) SetWriteDeadline(time.Time) error { return nil }

func TestProtoV1MessageRoundTrip(t *testing.T) {
	msg := &Message{
		ProtocolVersion: byte(Version1),
		Flags:           0x2A,
		Type:            byte(MessageTypeResponse),
		Payload:         []byte("hello"),
	}

	encoded := V1.EncodeMessage(msg)
	if want := 1 + 1 + 1 + 1 + 4 + len(msg.Payload); len(encoded) != want {
		t.Fatalf("encoded length mismatch: got %d, want %d", len(encoded), want)
	}

	decoded, err := V1.DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("DecodeMessage() unexpected error: %v", err)
	}
	if decoded.ProtocolVersion != msg.ProtocolVersion || decoded.Flags != msg.Flags || decoded.Type != msg.Type || !bytes.Equal(decoded.Payload, msg.Payload) {
		t.Fatalf("DecodeMessage() mismatch: got %+v, want %+v", decoded, msg)
	}

	var target Message
	if err := V1.DecodeMessageInto(encoded, &target); err != nil {
		t.Fatalf("DecodeMessageInto() unexpected error: %v", err)
	}
	if target.ProtocolVersion != msg.ProtocolVersion || target.Flags != msg.Flags || target.Type != msg.Type || !bytes.Equal(target.Payload, msg.Payload) {
		t.Fatalf("DecodeMessageInto() mismatch: got %+v, want %+v", target, *msg)
	}

	encodedInto := V1.EncodeMessageInto(msg, make([]byte, 0, 64))
	if !bytes.Equal(encodedInto, encoded) {
		t.Fatalf("EncodeMessageInto() mismatch: got %v, want %v", encodedInto, encoded)
	}
}

func TestProtoV1MessageErrors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{name: "empty", data: nil, want: ErrPayloadTooShort},
		{name: "magic", data: []byte{0x00, byte(Version1), 0x00, byte(MessageTypeCommand), 0, 0, 0, 1, 'x'}, want: ErrMagicNumberInvalid},
		{name: "version", data: []byte{magicNumber, 0x99, 0x00, byte(MessageTypeCommand), 0, 0, 0, 1, 'x'}, want: ErrInvalidProtocolVersion},
		{name: "type", data: []byte{magicNumber, byte(Version1), 0x00, 0xFF, 0, 0, 0, 1, 'x'}, want: ErrUnknownMessageType},
		{name: "empty payload", data: []byte{magicNumber, byte(Version1), 0x00, byte(MessageTypeCommand), 0, 0, 0, 0}, want: ErrEmptyPayload},
		{name: "short payload", data: []byte{magicNumber, byte(Version1), 0x00, byte(MessageTypeCommand), 0, 0, 0, 4}, want: ErrPayloadTooShort},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := V1.DecodeMessage(tc.data)
			if !errors.Is(err, tc.want) {
				t.Fatalf("DecodeMessage(%v) error = %v, want %v", tc.data, err, tc.want)
			}
		})
	}
}

func TestProtoV1CommandRoundTrip(t *testing.T) {
	cmd := &Command{ReqID: 0x01020304, ID: ServerCommandPing, Payload: []byte("ping")}
	encoded := V1.EncodeCommand(cmd)
	if want := 4 + 2 + 4 + len(cmd.Payload); len(encoded) != want {
		t.Fatalf("encoded length mismatch: got %d, want %d", len(encoded), want)
	}

	decoded, err := V1.DecodeCommand(&Message{Payload: encoded})
	if err != nil {
		t.Fatalf("DecodeCommand() unexpected error: %v", err)
	}
	if decoded.ReqID != cmd.ReqID || decoded.ID != cmd.ID || !bytes.Equal(decoded.Payload, cmd.Payload) {
		t.Fatalf("DecodeCommand() mismatch: got %+v, want %+v", decoded, cmd)
	}

	var target Command
	if err := V1.DecodeCommandInto(&Message{Payload: encoded}, &target); err != nil {
		t.Fatalf("DecodeCommandInto() unexpected error: %v", err)
	}
	if target.ReqID != cmd.ReqID || target.ID != cmd.ID || !bytes.Equal(target.Payload, cmd.Payload) {
		t.Fatalf("DecodeCommandInto() mismatch: got %+v, want %+v", target, *cmd)
	}

	if err := V1.DecodeCommandInto(&Message{Payload: []byte{0, 0, 0, 1, 0, 1}}, &Command{}); !errors.Is(err, ErrPayloadTooShort) {
		t.Fatalf("DecodeCommandInto() short payload error = %v, want %v", err, ErrPayloadTooShort)
	}
}

func TestProtoV1ResponseRoundTrip(t *testing.T) {
	resp := &Response{ReqID: 42, Code: StatusCodeOK, Payload: []byte("ok")}
	encoded := V1.EncodeResponse(resp)
	if want := 4 + 2 + 4 + len(resp.Payload); len(encoded) != want {
		t.Fatalf("encoded length mismatch: got %d, want %d", len(encoded), want)
	}

	decoded, err := V1.DecodeResponse(&Message{Payload: encoded})
	if err != nil {
		t.Fatalf("DecodeResponse() unexpected error: %v", err)
	}
	if decoded.ReqID != resp.ReqID || decoded.Code != resp.Code || !bytes.Equal(decoded.Payload, resp.Payload) {
		t.Fatalf("DecodeResponse() mismatch: got %+v, want %+v", decoded, resp)
	}

	var target Response
	if err := V1.DecodeResponseInto(&Message{Payload: encoded}, &target); err != nil {
		t.Fatalf("DecodeResponseInto() unexpected error: %v", err)
	}
	if target.ReqID != resp.ReqID || target.Code != resp.Code || !bytes.Equal(target.Payload, resp.Payload) {
		t.Fatalf("DecodeResponseInto() mismatch: got %+v, want %+v", target, *resp)
	}

	if err := V1.DecodeResponseInto(&Message{Payload: []byte{0, 0, 0, 1, 0, 1}}, &Response{}); !errors.Is(err, ErrPayloadTooShort) {
		t.Fatalf("DecodeResponseInto() short payload error = %v, want %v", err, ErrPayloadTooShort)
	}
}

func TestProtoV1FrameReadWrite(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	payload := []byte("frame-data")
	writeErrCh := make(chan error, 1)
	go func() {
		writeErrCh <- V1.WriteFrame(client, payload)
	}()

	readPayload, err := V1.ReadFrame(server)
	if err != nil {
		t.Fatalf("ReadFrame() unexpected error: %v", err)
	}
	if !bytes.Equal(readPayload, payload) {
		t.Fatalf("ReadFrame() mismatch: got %q, want %q", readPayload, payload)
	}
	if err := <-writeErrCh; err != nil {
		t.Fatalf("WriteFrame() unexpected error: %v", err)
	}

	client2, server2 := net.Pipe()
	defer client2.Close()
	defer server2.Close()
	go func() {
		_, _ = client2.Write([]byte{0, 0, 0, 0})
	}()
	if _, err := V1.ReadFrame(server2); !errors.Is(err, ErrFrameLengthInvalid) {
		t.Fatalf("ReadFrame() invalid length error = %v, want %v", err, ErrFrameLengthInvalid)
	}

	client3, server3 := net.Pipe()
	defer client3.Close()
	defer server3.Close()
	invalidLen := make([]byte, 4)
	binary.BigEndian.PutUint32(invalidLen, uint32(maxFrameSize)+1)
	go func() {
		_, _ = client3.Write(invalidLen)
	}()
	if _, err := V1.ReadFrame(server3); !errors.Is(err, ErrFrameLengthInvalid) {
		t.Fatalf("ReadFrame() oversized frame error = %v, want %v", err, ErrFrameLengthInvalid)
	}

	partial := &partialWriteConn{remainingShort: 3}
	payload2 := []byte("hello world")
	if err := V1.WriteFrame(partial, payload2); err != nil {
		t.Fatalf("WriteFrame() with short writes unexpected error: %v", err)
	}
	wantFrame := append([]byte{0, 0, 0, byte(len(payload2))}, payload2...)
	if len(partial.buf.Bytes()) != len(wantFrame) || !bytes.Equal(partial.buf.Bytes(), wantFrame) {
		t.Fatalf("WriteFrame() partial write mismatch: got %v, want %v", partial.buf.Bytes(), wantFrame)
	}
}

func TestObjectPoolLifecycle(t *testing.T) {
	buf := GetRequestBufferFromPool()
	buf = append(buf, 'a', 'b', 'c')
	PutRequestBufferToPool(buf)

	buf2 := GetRequestBufferFromPool()
	if len(buf2) != 0 {
		t.Fatalf("GetRequestBufferFromPool() returned len %d, want 0", len(buf2))
	}

	respBuf := GetResponseBufferFromPool()
	respBuf = append(respBuf, 'x', 'y')
	PutResponseBufferToPool(respBuf)
	respBuf2 := GetResponseBufferFromPool()
	if len(respBuf2) != 0 {
		t.Fatalf("GetResponseBufferFromPool() returned len %d, want 0", len(respBuf2))
	}

	msg := GetMessageFromPool()
	msg.ProtocolVersion = byte(Version1)
	msg.Flags = 0x1F
	msg.Type = byte(MessageTypeCommand)
	msg.Payload = []byte("payload")
	PutMessageToPool(msg)

	msg2 := GetMessageFromPool()
	if msg2.ProtocolVersion != 0 || msg2.Flags != 0 || msg2.Type != 0 || msg2.Payload != nil {
		t.Fatalf("PutMessageToPool()/GetMessageFromPool() reset mismatch: %+v", *msg2)
	}

	cmd := GetCommandFromPool()
	cmd.ReqID = 7
	cmd.ID = ServerCommandPing
	cmd.Payload = []byte("cmd")
	PutCommandToPool(cmd)

	cmd2 := GetCommandFromPool()
	if cmd2.ReqID != 0 || cmd2.ID != 0 || cmd2.Payload != nil {
		t.Fatalf("PutCommandToPool()/GetCommandFromPool() reset mismatch: %+v", *cmd2)
	}
}

func TestProtoV1EncodeDecodeMessageWithTargetReuse(t *testing.T) {
	target := make([]byte, 64)
	msg := &Message{ProtocolVersion: byte(Version1), Flags: 0x12, Type: byte(MessageTypeCommand), Payload: []byte("target")}
	encoded := V1.EncodeMessageInto(msg, target)
	if got, want := len(encoded), 1+1+1+1+4+len(msg.Payload); got != want {
		t.Fatalf("len(encoded) = %d, want %d", got, want)
	}

	decoded, err := V1.DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("DecodeMessage() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(decoded.Payload, msg.Payload) {
		t.Fatalf("DecodeMessage() payload mismatch: got %q, want %q", decoded.Payload, msg.Payload)
	}
}
