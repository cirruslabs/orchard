package responder

import (
	"github.com/gin-gonic/gin"
)

type JSONResponder struct {
	code    int
	headers map[string]string
	obj     interface{}

	DefaultResponder
}

func JSON(code int, obj interface{}, opts ...Option) *JSONResponder {
	responder := &JSONResponder{
		code:    code,
		headers: map[string]string{},
		obj:     obj,
	}

	for _, opt := range opts {
		opt(responder)
	}

	return responder
}

func (responder *JSONResponder) SetHeader(key string, value string) {
	responder.headers[key] = value
}

func (responder *JSONResponder) Respond(c *gin.Context) {
	for key, value := range responder.headers {
		c.Header(key, value)
	}

	c.JSON(responder.code, responder.obj)
}
