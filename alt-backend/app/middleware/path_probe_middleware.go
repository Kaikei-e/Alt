package middleware

import (
	"github.com/labstack/echo/v4"
)

// PathProbe middleware for detecting path modifications during request processing
// This is a temporary debugging middleware to identify where paths get changed to "/"
func PathProbe(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		r := c.Request()
		
		// Set arrival path headers for debugging
		c.Response().Header().Set("X-Req-Path", r.URL.Path) // Path on arrival
		if r.URL.RawPath != "" {
			c.Response().Header().Set("X-Req-RawPath", r.URL.RawPath)
		}
		c.Response().Header().Set("X-Echo-Path", c.Path()) // Path after Echo routing
		
		// Add original URI for reference
		c.Response().Header().Set("X-Original-URI", r.RequestURI)
		
		return next(c)
	}
}