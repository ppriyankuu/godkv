package api

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

////////////////////////////////////////////////////////////////////////////////
// REQUEST LOGGER MIDDLEWARE
////////////////////////////////////////////////////////////////////////////////

// Logger logs every incoming HTTP request.
//
// Why do we need logging?
//
// In distributed systems you MUST know:
//
//   - What request came in?
//   - From which IP?
//   - What path?
//   - What status code did we return?
//   - How long did it take?
//
// Without logging:
//
//	→ Debugging becomes painful.
//	→ Production issues are invisible.
//
// This middleware prints structured logs for each request.
func Logger() gin.HandlerFunc {

	// Gin middleware always returns a function
	// that receives the request context.
	return func(c *gin.Context) {

		// Record start time.
		// We'll use this to calculate latency.
		start := time.Now()

		// c.Next() executes the next handler
		// in the middleware chain.
		//
		// Important:
		// Everything BEFORE c.Next() runs before the handler.
		// Everything AFTER c.Next() runs after the handler.
		c.Next()

		// Calculate how long the request took.
		latency := time.Since(start)

		// Log useful request details.
		log.Printf("[%s] %s %s | %d | %s",
			c.Request.Method,   // GET, PUT, DELETE, etc.
			c.Request.URL.Path, // /kv/mykey
			c.ClientIP(),       // client IP address
			c.Writer.Status(),  // HTTP status code (200, 404, 500)
			latency,            // total processing time
		)
	}
}

////////////////////////////////////////////////////////////////////////////////
// PANIC RECOVERY MIDDLEWARE
////////////////////////////////////////////////////////////////////////////////

// Recovery protects the server from crashing due to panics.
//
// What is a panic?
//
// A panic is a runtime crash inside your Go program.
// Example causes:
//   - nil pointer dereference
//   - out-of-bounds slice access
//   - unexpected programming bug
//
// If we DO NOT recover:
//
//	→ The entire server crashes.
//	→ All clients lose connection.
//	→ In distributed systems, this is very bad.
//
// This middleware catches panics and:
//  1. Logs the error
//  2. Returns HTTP 500
//  3. Prevents server crash
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {

		// defer runs when this function exits.
		// We use it to catch panics.
		defer func() {

			// recover() captures a panic if one occurred.
			if err := recover(); err != nil {

				// Log the panic.
				log.Printf("PANIC recovered: %v", err)

				// Abort the request.
				// We return a safe generic error to the client.
				// Never expose internal panic details in production.
				c.AbortWithStatusJSON(500, gin.H{
					"error": "internal server error",
				})
			}
		}()

		// Continue execution.
		// If a panic happens inside later handlers,
		// our defer block above will catch it.
		c.Next()
	}
}
