package responder

import (
	"github.com/gin-gonic/gin"
)

type Responder interface {
	Respond(c *gin.Context)
}

type DefaultResponder struct{}

func (dr DefaultResponder) Respond(c *gin.Context) {}
