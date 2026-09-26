package engine

import (
	"context"
	"net/http"
	"strings"
)

// Router defines the routing logic for incoming requests.
type Router struct {
	routes []route
}

type route struct {
	method  string
	pattern string
	handler http.Handler
}

func NewRouter() *Router {
	return &Router{}
}

func (r *Router) Handle(method, pattern string, handler http.Handler) {
	r.routes = append(r.routes, route{method: strings.ToUpper(method), pattern: pattern, handler: handler})
}

func (r *Router) EndpointLabel(method, path string) string {
	for _, candidate := range r.routes {
		if candidate.method == method {
			if _, matches := matchPath(candidate.pattern, path); matches {
				return candidate.pattern
			}
		}
	}
	return "unmatched"
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	pathMatched := false
	for _, candidate := range r.routes {
		params, matches := matchPath(candidate.pattern, req.URL.Path)
		if !matches {
			continue
		}
		pathMatched = true
		if candidate.method != req.Method {
			continue
		}
		candidate.handler.ServeHTTP(w, req.WithContext(contextWithPathParams(req, params)))
		return
	}
	if pathMatched {
		w.Header().Set("Allow", allowedMethods(r.routes, req.URL.Path))
		writeAPIError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", requestIDFrom(req))
		return
	}
	writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "route not found", requestIDFrom(req))
}

type pathParamsKey struct{}

func contextWithPathParams(req *http.Request, params map[string]string) context.Context {
	return context.WithValue(req.Context(), pathParamsKey{}, params)
}

func pathParams(ctx context.Context) map[string]string {
	params, _ := ctx.Value(pathParamsKey{}).(map[string]string)
	return params
}

func matchPath(pattern, path string) (map[string]string, bool) {
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	if pattern == "/" {
		patternParts, pathParts = nil, nil
	}
	if len(patternParts) != len(pathParts) {
		return nil, false
	}
	params := make(map[string]string)
	for index, part := range patternParts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
			if name == "" || pathParts[index] == "" {
				return nil, false
			}
			params[name] = pathParts[index]
			continue
		}
		if part != pathParts[index] {
			return nil, false
		}
	}
	return params, true
}

func allowedMethods(routes []route, path string) string {
	methods := make(map[string]bool)
	var values []string
	for _, candidate := range routes {
		if _, matches := matchPath(candidate.pattern, path); matches && !methods[candidate.method] {
			methods[candidate.method] = true
			values = append(values, candidate.method)
		}
	}
	return strings.Join(values, ", ")
}
