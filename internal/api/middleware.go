package api

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger is a Gin middleware that logs every request with method, path,
// status code, and latency.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("[%s] %s %s | %d | %s",
			c.Request.Method,
			c.Request.URL.Path,
			c.ClientIP(),
			c.Writer.Status(),
			time.Since(start),
		)
	}
}

// Recovery wraps Gin's default recovery but logs panics in a structured way.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC recovered: %v", err)
				c.AbortWithStatusJSON(500, gin.H{"error": "internal server error"})
			}
		}()
		c.Next()
	}
}
