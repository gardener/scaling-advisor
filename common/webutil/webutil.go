// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package webutil

import (
	"net/http"

	"github.com/go-logr/logr"
)

// LoggerMiddleware creates a middleware that injects the logger into the request context.
func LoggerMiddleware(log logr.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Inject the log into the request's context
		ctx := logr.NewContext(r.Context(), log.WithValues("method", r.Method, "requestURI", r.RequestURI))
		// Create a new request with the updated context
		r = r.WithContext(ctx)
		// Call the next handler with the modified request
		next.ServeHTTP(w, r)
	})
}
