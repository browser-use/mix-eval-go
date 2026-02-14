//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"mix-eval-go/pkg/convex"
)

func TestPostHogDatasetExists(t *testing.T) {
	// Setup
	convexURL := os.Getenv("CONVEX_URL")
	secretKey := os.Getenv("CONVEX_SECRET_KEY")

	if convexURL == "" || secretKey == "" {
		t.Skip("CONVEX_URL and CONVEX_SECRET_KEY required for E2E tests")
	}

	client := convex.NewClient(convexURL, secretKey)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute
	tasks, err := client.FetchTestCase(ctx, "PostHog_Cleaned_020226")

	// Assert
	if err != nil {
		t.Fatalf("Failed to fetch PostHog_Cleaned_020226 dataset: %v", err)
	}

	// Verify task count
	expectedCount := 333
	if len(tasks) != expectedCount {
		t.Errorf("Expected %d tasks, got %d", expectedCount, len(tasks))
	}

	// Verify tasks have required fields
	if len(tasks) > 0 {
		firstTask := tasks[0]

		if firstTask.ID == "" {
			t.Error("First task missing task_id")
		}

		if firstTask.Text == "" {
			t.Error("First task missing confirmed_task/task text")
		}

		t.Logf("✓ Dataset verified: PostHog_Cleaned_020226 with %d tasks", len(tasks))
		t.Logf("  Sample task ID: %s", firstTask.ID)
	} else {
		t.Error("Dataset returned zero tasks")
	}
}

func TestDatasetConnectivity(t *testing.T) {
	// Test basic Convex API connectivity
	convexURL := os.Getenv("CONVEX_URL")
	secretKey := os.Getenv("CONVEX_SECRET_KEY")

	if convexURL == "" || secretKey == "" {
		t.Skip("CONVEX_URL and CONVEX_SECRET_KEY required for E2E tests")
	}

	client := convex.NewClient(convexURL, secretKey)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try fetching known test cases
	testCases := []struct {
		name         string
		minTaskCount int
	}{
		{"InteractionTasks_v6", 1},
		{"BUTasksv0", 1},
		{"PostHog_Cleaned_020226", 333},
	}

	for _, tc := range testCases {
		tasks, err := client.FetchTestCase(ctx, tc.name)
		if err != nil {
			t.Logf("⚠ Dataset %s not accessible: %v", tc.name, err)
			continue
		}

		if len(tasks) < tc.minTaskCount {
			t.Errorf("Dataset %s has %d tasks, expected at least %d", tc.name, len(tasks), tc.minTaskCount)
		} else {
			t.Logf("✓ Dataset %s accessible (%d tasks)", tc.name, len(tasks))
		}
	}
}
