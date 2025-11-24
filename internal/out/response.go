package out

import (
	"fmt"
	"net/http"
)

type Response struct {
	ContentType string
	StatusCode  int
	Body        any
	Headers     http.Header
	Cookies     []*http.Cookie
}

func (r *Response) Error() string {
	return fmt.Sprintf("responding with: %d", r.StatusCode)
}
