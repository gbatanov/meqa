package mqutil

import (
	"fmt"
)

const (
	ErrOK         = iota // 0
	ErrInvalid           // invalid parameters
	ErrNotFound          // resource not found
	ErrExpect            // the REST result doesn't match the expected value
	ErrHttp              // Http request failed
	ErrServerResp        // unexpected server response
	ErrInternal          // unexpected internal error (meqa error)
)

// Error implements MQ specific error type.
type Error interface {
	error
	Type() int
}

// TypedError holds a type and a back trace for easy debugging
type TypedError struct {
	errType int
	errMsg  string
}

func (e *TypedError) Error() string {
	return e.errMsg
}

func (e *TypedError) Type() int {
	return e.errType
}
func (e *TypedError) TypeString() string {
	switch e.errType {
	case ErrOK:
		return "success"
	case ErrInvalid:
		return "invalid parameters"
	case ErrNotFound:
		return "resource not found"
	case ErrExpect:
		return "the REST result doesn't match the expected value"
	case ErrHttp:
		return "Http request failed"
	case ErrServerResp:
		return "unexpected server response"
	case ErrInternal:
		return "unexpected internal error (meqa error)"
	default:
		return "unknown error"
	}
}

func NewError(errType int, str string) error {
	err := TypedError{errType, ""}
	err.errMsg = fmt.Sprintf("==== %s ====\nError message:\n%s\n", err.TypeString(), str)
	return &err
}
