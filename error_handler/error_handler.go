package error_handler

import (
	"net/http"
	"os"
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

		msg := err.Error()
		if os.Getenv("MODE") == "release" {
			msg = "Internal Server Error"
		}

		c.AbortWithStatusJSON(code, tools.Response{
			Status:  "error",
			Message: msg,
		})
		return
	}

	c.AbortWithStatusJSON(code, tools.Response{
		Status:  "error",
		Message: err.Error(),
	})
}
