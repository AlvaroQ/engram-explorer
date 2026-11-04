package httpapi

import "net/http"

// routeMux is what the route-group functions register on: *http.ServeMux, or
// a capabilityMux that gates every route.
type routeMux interface {
	Handle(pattern string, h http.Handler)
	HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request))
}
