//nolint:testpackage // we need to test unexported exec helpers
package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseExecInteractive(t *testing.T) {
	for _, test := range []struct {
		name        string
		query       string
		interactive bool
		errContains string
	}{
		{
			name:        "default false",
			interactive: false,
		},
		{
			name:        "interactive true",
			query:       "interactive=true",
			interactive: true,
		},
		{
			name:        "stdin alias true",
			query:       "stdin=true",
			interactive: true,
		},
		{
			name:        "matching values accepted",
			query:       "interactive=true&stdin=true",
			interactive: true,
		},
		{
			name:        "conflicting values rejected",
			query:       "interactive=true&stdin=false",
			errContains: "cannot conflict",
		},
		{
			name:        "invalid interactive rejected",
			query:       "interactive=maybe",
			errContains: "interactive",
		},
		{
			name:        "invalid stdin rejected",
			query:       "stdin=maybe",
			errContains: "stdin",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			interactive, err := parseExecInteractive(execQueryContext(test.query))
			if test.errContains != "" {
				require.ErrorContains(t, err, test.errContains)

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.interactive, interactive)
		})
	}
}

func execQueryContext(query string) *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)

	return ctx
}
