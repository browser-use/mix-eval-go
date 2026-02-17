//go:build e2e

package e2e

import (
	"os"
	"testing"

	"github.com/joho/godotenv"
)

func TestMain(m *testing.M) {
	if err := godotenv.Load("../../.env"); err != nil {
		// Not fatal — CI may inject env vars directly
		_ = err
	}

	os.Exit(m.Run())
}
