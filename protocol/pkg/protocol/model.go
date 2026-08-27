package protocol

const magicNumber byte = 0x67

type Message struct {
	ProtocolVersion byte
	Flags           byte
	Type            byte
	Payload         []byte
}

type MessageType byte

const (
	MessageTypeCommand  MessageType = 0x01
	MessageTypeResponse MessageType = 0x02
)

type Version byte

const (
	Version1 Version = 0x01
)

type Command struct {
	ReqID   uint32
	ID      uint16
	Payload []byte
}

type Response struct {
	ReqID   uint32
	Code    uint16
	Payload []byte
}
