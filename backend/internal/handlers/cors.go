package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// corsAllowedOrigins is the explicit allowlist of browser origins permitted to
// call the API. We never use "*" so credentials and origins stay controlled.
var corsAllowedOrigins = map[string]bool{
	"http://localhost:5173": true, // SvelteKit dev server
	"http://localhost:4173": true, // SvelteKit preview
}

// CORS returns middleware that sets cross-origin headers for allowed origins
// and short-circuits preflight (OPTIONS) requests.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if corsAllowedOrigins[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
