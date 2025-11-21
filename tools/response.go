package tools

type Response struct {
	Status   string                 `json:"status,omitempty"`
	Message  string                 `json:"message,omitempty"`
	Messages map[string]interface{} `json:"messages,omitempty"`
	Data     interface{}            `json:"data,omitempty"`
	Meta     interface{}            `json:"meta,omitempty"`
}
