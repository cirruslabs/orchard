package responder

import (
	"github.com/gin-gonic/gin"
)

type CodeResponder struct {
	code int
	Responder
}

func Code(code int) Responder {
	return &CodeResponder{
		code: code,
	}
}

func (responder *CodeResponder) Respond(c *gin.Context) {
	c.Status(responder.code)
}
