package request

import (
	"fmt"
	"net/http"

	"github.com/rnd-varnion/utils/request/action"
)

var (
	METHOD [5]string = [5]string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPatch,
		http.MethodPut,
		http.MethodDelete,
	}
)

func Client(url string, method string, payload []byte) ([]byte, int, error) {
	switch method {
	case METHOD[0]:
		return action.GET(url)
	case METHOD[1]:
		return action.POST(url, payload)
	case METHOD[2]:
		return action.PATCH(url, payload)
	case METHOD[3]:
		return action.PUT(url, payload)
	case METHOD[4]:
		return action.DELETE(url)
	}

	return nil, http.StatusNotFound, fmt.Errorf("method not found in library")
}
