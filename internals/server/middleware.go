package server

import "net/http"



func (s *Server) AddMiddlewaresToHandler(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	var handler http.Handler =h
	
	// Wrap the handler with middlewares (iterate backwards)
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	return handler
}