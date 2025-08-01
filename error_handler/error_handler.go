package error_handler

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
	"github.com/rnd-varnion/utils/logger"
	"github.com/rnd-varnion/utils/tools"
)

func HandleError(c *gin.Context, code int, err error) {
	if code == http.StatusInternalServerError {
		funcName := ""
		if pc, _, _, ok := runtime.Caller(8); ok {
			funcName = runtime.FuncForPC(pc).Name()
		}
		logger.Log.WithField("func", funcName).Error(err)

		c.AbortWithStatusJSON(code, tools.Response{
			Status:  "error",
			Message: "Internal Server Error",
		})
		return
	}

	c.AbortWithStatusJSON(code, tools.Response{
		Status:  "error",
		Message: err.Error(),
	})
}
