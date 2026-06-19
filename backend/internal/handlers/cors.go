package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS returns middleware that sets cross-origin headers for allowed origins
// and short-circuits preflight (OPTIONS) requests. The allow-list is passed in
// (from config) so we never fall back to a wildcard "*".
func CORS(allowed map[string]bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if allowed[origin] {
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
