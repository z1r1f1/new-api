package middleware

import (
	"github.com/gin-gonic/gin"
)

func Cache() func(c *gin.Context) {
	return func(c *gin.Context) {
		// Always revalidate embedded web assets after a container rebuild.
		// The frontend already emits hashed file names, but browsers can still
		// display broken layouts when a stale cached JS/CSS pair is mixed with
		// the freshly served index.html during local image rebuilds.
		c.Header("Cache-Control", "no-cache")
		c.Header("Cache-Version", "b688f2fb5be447c25e5aa3bd063087a83db32a288bf6a4f35f2d8db310e40b14")
		c.Next()
	}
}
