package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"mix-eval-go/pkg/orchestrator"
)

func main() {
	// Parse command line flags
	testCaseName := flag.String("test-case", "", "Test case name to run")
	runID := flag.String("run-id", "", "Run ID for this evaluation")
	parallelism := flag.Int("parallel", 3, "Number of parallel tasks")
	flag.Parse()

	if *testCaseName == "" {
		log.Fatal("--test-case is required")
	}

	// Load configuration from environment
	config := orchestrator.Config{
		MixURL:          getEnv("MIX_AGENT_URL", "http://localhost:8088"),
		ConvexURL:       getEnv("CONVEX_URL", ""),
		ConvexSecretKey: getEnv("CONVEX_SECRET_KEY", ""),
		BrowserbaseKey:  getEnv("BROWSERBASE_API_KEY", ""),
		BrightdataUser:  getEnv("BRIGHTDATA_USER", ""),
		BrightdataPass:  getEnv("BRIGHTDATA_PASS", ""),
	}

	// Validate required config
	if config.ConvexURL == "" || config.ConvexSecretKey == "" {
		log.Fatal("CONVEX_URL and CONVEX_SECRET_KEY are required")
	}

	// Create orchestrator
	orch := orchestrator.New(config)

	ctx := context.Background()

	// Fetch tasks from Convex
	fmt.Printf("Fetching test case: %s\n", *testCaseName)
	tasks, err := orch.FetchTasks(ctx, *testCaseName)
	if err != nil {
		log.Fatalf("Failed to fetch tasks: %v", err)
	}

	fmt.Printf("Found %d tasks\n", len(tasks))

	// Set run ID on all tasks
	if *runID != "" {
		for i := range tasks {
			tasks[i].RunID = *runID
		}
	}

	// Run tasks in parallel
	fmt.Printf("Starting evaluation with parallelism=%d\n", *parallelism)
	if err := orch.RunMultipleTasks(ctx, tasks, *parallelism); err != nil {
		log.Fatalf("Execution failed: %v", err)
	}

	fmt.Println("✅ All tasks completed successfully")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
