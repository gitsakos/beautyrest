package beautyrest

import "net/http"

type AuthInterface interface {
	MakeFromRequest(r *http.Request) (interface{}, error)
}
