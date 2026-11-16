package httpserver

import (
	"crypto/subtle"
	"net/http"
)

func validToken(r *http.Request, token string) bool {
	provided := r.Header.Get("X-Temper-Token")
	return subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}
