package server

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	// Set gin mode once before all parallel tests to avoid a data race on
	// gin's global ginMode variable (gin.SetMode is not concurrency-safe).
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}
