package controller

import "fmt"

type Error struct {
	Message string `json:"message"`
}

func NewError(format string, args ...any) *Error {
	return &Error{
		Message: fmt.Sprintf(format, args...),
	}
}
