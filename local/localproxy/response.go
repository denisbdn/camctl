package localproxy

type RespType int

const (
	OK       RespType = 0
	NotFound          = 1
)

// Response is describe out json
type Response struct {
	Errno RespType    `json:"errno,omitempty"`
	Error string      `json:"error,omitempty"`
	Data  interface{} `json:"data,omitempty"`
}
