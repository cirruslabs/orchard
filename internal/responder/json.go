package responder

import (
	"github.com/gin-gonic/gin"
)

type JSONResponder struct {
	code int
	obj  interface{}

	Responder
}

func JSON(code int, obj interface{}) *JSONResponder {
	responder := &JSONResponder{
		code: code,
		obj:  obj,
	}

	return responder
}
func (responder *JSONResponder) Respond(c *gin.Context) {
	c.JSON(responder.code, responder.obj)
}
