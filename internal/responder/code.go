package responder

import "github.com/gin-gonic/gin"

type CodeResponder struct {
	code    int
	headers map[string]string

	DefaultResponder
}

func Code(code int, opts ...Option) *CodeResponder {
	responder := &CodeResponder{
		code:    code,
		headers: map[string]string{},
	}

	for _, opt := range opts {
		opt(responder)
	}

	return responder
}

func (responder *CodeResponder) SetHeader(key string, value string) {
	responder.headers[key] = value
}

func (responder *CodeResponder) Respond(c *gin.Context) {
	for key, value := range responder.headers {
		c.Header(key, value)
	}

	c.Status(responder.code)
}
