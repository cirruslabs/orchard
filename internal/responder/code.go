package responder

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

type CodeResponder struct {
	code int
	err  error
	DefaultResponder
}

func Code(code int) *CodeResponder {
	return ErrorCode(nil, code)
}

func Error(err error) *CodeResponder {
	return ErrorCode(err, http.StatusInternalServerError)
}

func ErrorCode(err error, code int) *CodeResponder {
	responder := &CodeResponder{
		code: code,
		err:  err,
	}

	return responder
}

func (responder *CodeResponder) Respond(c *gin.Context) {
	c.Status(responder.code)
	if responder.err != nil {
		_ = c.Error(responder.err)
	}
}
