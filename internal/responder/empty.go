package responder

import "github.com/gin-gonic/gin"

type EmptyResponder struct{}

func Empty() *EmptyResponder {
	return &EmptyResponder{}
}

func (responder *EmptyResponder) Respond(c *gin.Context) {
	// do nothing
}
