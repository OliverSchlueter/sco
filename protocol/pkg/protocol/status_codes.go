package protocol

// Status codes
// 200 - 299 Success
// 400 - 499 Client Error
// 500 - 599 Server Error

const (
	StatusCodeOK uint16 = 200

	StatusInternalError   uint16 = 500
	StatusCommandNotFound uint16 = 501
)
