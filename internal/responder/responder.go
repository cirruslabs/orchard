package responder

import (
	"github.com/gin-gonic/gin"
)

type Responder interface {
	Respond(c *gin.Context)
	SetHeader(key string, value string)
}

type DefaultResponder struct{}

func (dr DefaultResponder) Respond(c *gin.Context)             {}
func (dr DefaultResponder) SetHeader(key string, value string) {}
