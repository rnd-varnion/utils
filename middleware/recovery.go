package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/rnd-varnion/utils/logger"
	"github.com/rnd-varnion/utils/tools"
)

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("%v", r)

				logger.Log.WithError(err).WithField("stack", string(debug.Stack())).Errorf("panic recovered: %v", r)

				c.AbortWithStatusJSON(http.StatusInternalServerError, tools.Response{
					Status:  "error",
					Message: "Internal Server Error",
				})
			}
		}()
		c.Next()
	}
}
