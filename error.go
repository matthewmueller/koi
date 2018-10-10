package koi

// OutputError is a error result of a function call.
type OutputError struct {
	Error Error `json:"error"`
}

// Error is an koi error.
type Error struct {
	Status  int    `json:"-"`
	Type    string `json:"type,omitempty"`
	Message string `json:"message"`
}

// NewError returns a new RPC error with HTTP status code, kind and message.
func NewError(status int, kind, message string) error {
	return Error{
		Status:  status,
		Type:    kind,
		Message: message,
	}
}

// Error implementation.
func (e Error) Error() string {
	return e.Message
}
