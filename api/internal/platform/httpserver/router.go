package httpserver

import (
	"net/http"
	"strings"
)

type Router struct {
	token  string
	routes []route
}

type route struct {
	method   string
	segments []string
	handler  http.Handler
}

func NewRouter(token string) *Router {
	return &Router{token: token}
}

func (r *Router) Handle(pattern string, handler http.Handler) {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok || method == "" || !strings.HasPrefix(path, "/") {
		panic("invalid route pattern: " + pattern)
	}
	r.routes = append(r.routes, route{
		method:   method,
		segments: pathSegments(path),
		handler:  handler,
	})
}

func (r *Router) HandleFunc(pattern string, fn http.HandlerFunc) {
	r.Handle(pattern, fn)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet && request.URL.Path == "/api/health" {
		WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	if !validToken(request, r.token) {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token")
		return
	}

	for _, candidate := range r.routes {
		if values, ok := candidate.match(request.Method, request.URL.Path); ok {
			for name, value := range values {
				request.SetPathValue(name, value)
			}
			candidate.handler.ServeHTTP(w, request)
			return
		}
	}

	WriteError(w, http.StatusNotFound, "not_found", "route not found")
}

func (r route) match(method, path string) (map[string]string, bool) {
	if r.method != method {
		return nil, false
	}

	parts := pathSegments(path)
	if len(parts) != len(r.segments) {
		return nil, false
	}

	values := make(map[string]string)
	for index, segment := range r.segments {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			name := segment[1 : len(segment)-1]
			if name == "" || parts[index] == "" {
				return nil, false
			}
			values[name] = parts[index]
			continue
		}
		if segment != parts[index] {
			return nil, false
		}
	}

	return values, true
}

func pathSegments(path string) []string {
	if path == "/" {
		return nil
	}
	return strings.Split(strings.TrimPrefix(path, "/"), "/")
}
