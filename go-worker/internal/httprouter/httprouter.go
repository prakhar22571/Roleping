// Package httprouter is a minimal method+path router.
//
// It exists because TinyGo's bundled net/http ships an older ServeMux that
// predates Go 1.22's enhanced "METHOD /path/{param}" pattern syntax - patterns
// like "GET /api/companies" are treated as literal (unmatchable) path strings
// under TinyGo, not parsed as method+wildcard patterns. This router
// implements just enough of that syntax (method prefix, {name} segments) by
// hand so the rest of the codebase can use familiar route definitions.
package httprouter

import (
	"context"
	"net/http"
	"strings"
)

type route struct {
	method   string
	segments []string
	handler  http.HandlerFunc
}

type Router struct {
	routes []route
}

func New() *Router {
	return &Router{}
}

func splitPath(p string) []string {
	trimmed := strings.Trim(p, "/")
	if trimmed == "" {
		return []string{}
	}
	return strings.Split(trimmed, "/")
}

// Handle registers a handler for a "METHOD /path/{param}" pattern.
func (rt *Router) Handle(pattern string, handler http.HandlerFunc) {
	parts := strings.SplitN(pattern, " ", 2)
	if len(parts) != 2 {
		panic("httprouter: pattern must be \"METHOD /path\": " + pattern)
	}
	rt.routes = append(rt.routes, route{
		method:   parts[0],
		segments: splitPath(parts[1]),
		handler:  handler,
	})
}

type contextKey string

const paramsContextKey contextKey = "httprouter.params"

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqSegments := splitPath(r.URL.Path)

	for _, rte := range rt.routes {
		if rte.method != r.Method || len(rte.segments) != len(reqSegments) {
			continue
		}

		params := map[string]string{}
		matched := true
		for i, seg := range rte.segments {
			if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
				params[seg[1:len(seg)-1]] = reqSegments[i]
				continue
			}
			if seg != reqSegments[i] {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}

		ctx := context.WithValue(r.Context(), paramsContextKey, params)
		rte.handler(w, r.WithContext(ctx))
		return
	}

	http.NotFound(w, r)
}

// PathValue returns the named path parameter captured by a {name} segment.
func PathValue(r *http.Request, name string) string {
	params, _ := r.Context().Value(paramsContextKey).(map[string]string)
	return params[name]
}
