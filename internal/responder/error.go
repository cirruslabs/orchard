package responder

import (
	"errors"
	storepkg "github.com/cirruslabs/orchard/internal/controller/store"
	"github.com/gin-gonic/gin"
	"net/http"
)

type ErrorResponder struct {
	err error
	Responder
}

func Error(err error) Responder {
	return &ErrorResponder{
		err: err,
	}
}

func (responder *ErrorResponder) Respond(c *gin.Context) {
	var code = http.StatusInternalServerError

	if errors.Is(responder.err, storepkg.ErrNotFound) {
		code = http.StatusNotFound
	} else {
		_ = c.Error(responder.err)
	}

	c.Status(code)
}
