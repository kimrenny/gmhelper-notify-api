package middleware

import "net/http"

type Middleware func(http.Handler) http.Handler

func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// Combine merges multiple middlewares into a single Middleware applied in sequential order.
func Combine(middlewares ...Middleware) Middleware {
	return func(finalHandler http.Handler) http.Handler {
		return Chain(finalHandler, middlewares...)
	}
}
