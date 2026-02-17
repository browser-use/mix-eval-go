package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	"mix-eval-go/pkg/orchestrator"
)

func main() {
	// Auto-load .env file if present (silently ignore if missing)
	_ = godotenv.Load()

	// Parse command line flags
	datasetName := flag.String("dataset", "", "Dataset name to run (required)")
	taskID := flag.String("task-id", "", "Specific task ID to run (optional, overrides index range)")
	runID := flag.String("run-id", "", "Run ID for this evaluation")
	startIndex := flag.Int("start-index", 0, "Start index for task range (inclusive)")
	endIndex := flag.Int("end-index", -1, "End index for task range (inclusive, -1 for all)")
	parallelism := flag.Int("parallel", 3, "Number of parallel tasks")
	browserProvider := flag.String("browser-provider", "", "Browser provider (browserbase, brightdata, hyperbrowser, anchor)")
	model := flag.String("model", "", "LLM model to use (overrides task default)")
	maxSteps := flag.Int("max-steps", 0, "Maximum steps per task (0 for no limit)")
	flag.Parse()

	if *datasetName == "" {
		log.Fatal("--dataset is required")
	}

	// Load configuration from environment
	config := orchestrator.Config{
		MixURL:          getEnv("MIX_AGENT_URL", "http://localhost:8088"),
		ConvexURL:       getEnv("CONVEX_URL", ""),
		ConvexSecretKey: getEnv("CONVEX_SECRET_KEY", ""),
		BrowserbaseKey:  getEnv("BROWSERBASE_API_KEY", ""),
		BrightdataUser:  getEnv("BRIGHTDATA_USER", ""),
		BrightdataPass:  getEnv("BRIGHTDATA_PASS", ""),
		GeminiAPIKey: getEnv("GEMINI_API_KEY", ""),
	}

	// Validate required config
	if config.ConvexURL == "" || config.ConvexSecretKey == "" {
		log.Fatal("CONVEX_URL and CONVEX_SECRET_KEY are required")
	}
	if config.GeminiAPIKey == "" {
		log.Fatal("GEMINI_API_KEY is required for judge evaluation")
	}

	// Create orchestrator
	orch := orchestrator.New(config)

	ctx := context.Background()

	// Fetch tasks from Convex
	fmt.Printf("Fetching dataset: %s\n", *datasetName)
	tasks, err := orch.FetchTasks(ctx, *datasetName)
	if err != nil {
		log.Fatalf("Failed to fetch tasks: %v", err)
	}

	fmt.Printf("Found %d tasks\n", len(tasks))

	// Handle task-id or index range
	if *taskID != "" {
		// Find task by ID
		taskIndex := -1
		for i, task := range tasks {
			if task.ID == *taskID {
				taskIndex = i
				break
			}
		}

		if taskIndex == -1 {
			log.Fatalf("Task ID %s not found in dataset", *taskID)
		}

		fmt.Printf("Found task %s at index %d\n", *taskID, taskIndex)
		tasks = tasks[taskIndex : taskIndex+1]
		fmt.Printf("Processing task: %s\n", *taskID)
	} else {
		// Apply index range filter
		if *endIndex == -1 {
			*endIndex = len(tasks) - 1
		}
		if *startIndex < 0 || *startIndex >= len(tasks) {
			log.Fatalf("Invalid start-index: %d (must be 0-%d)", *startIndex, len(tasks)-1)
		}
		if *endIndex < *startIndex || *endIndex >= len(tasks) {
			log.Fatalf("Invalid end-index: %d (must be %d-%d)", *endIndex, *startIndex, len(tasks)-1)
		}

		tasks = tasks[*startIndex : *endIndex+1]
		fmt.Printf("Processing tasks %d-%d (%d tasks)\n", *startIndex, *endIndex, len(tasks))
	}

	// Set run ID on all tasks (auto-generate if not provided)
	if *runID == "" {
		*runID = fmt.Sprintf("run-%d", time.Now().Unix())
		fmt.Printf("Auto-generated run ID: %s\n", *runID)
	}
	for i := range tasks {
		tasks[i].RunID = *runID
	}

	// Apply model override if specified
	if *model != "" {
		fmt.Printf("Overriding model to: %s\n", *model)
		// Note: Model override would be implemented in orchestrator
	}

	// Apply max steps if specified
	if *maxSteps > 0 {
		fmt.Printf("Setting max steps: %d\n", *maxSteps)
		// Note: Max steps would be implemented in orchestrator
	}

	// Apply browser provider if specified
	if *browserProvider != "" {
		fmt.Printf("Using browser provider: %s\n", *browserProvider)
		// Note: Browser provider would be passed to orchestrator config
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
