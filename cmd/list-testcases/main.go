package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// TestCase represents a test case from Convex
type TestCase struct {
	ID           string `json:"_id"`
	Name         string `json:"name"`
	NumberOfTasks int    `json:"numberOfTasks"`
	IsEnabled    *bool  `json:"isEnabled,omitempty"`
	IsTodoDataset bool   `json:"isTodoDataset,omitempty"`
}

func main() {
	convexURL := os.Getenv("CONVEX_URL")
	secretKey := os.Getenv("CONVEX_SECRET_KEY")

	if convexURL == "" || secretKey == "" {
		log.Fatal("CONVEX_URL and CONVEX_SECRET_KEY environment variables are required")
	}

	// Try to fetch some common test case names to see what's available
	// Since there's no listTestCases endpoint, we'll try common names
	commonNames := []string{
		"InteractionTasks_v6",
		"BUTasksv0",
		"proprietary_v1",
		"public_basic",
		"MixEvalTasks",
		"ManusEvalTasks",
	}

	fmt.Println("Attempting to fetch test cases from evaluation-platform...")
	fmt.Println("=" + fmt.Sprintf("%60s", "=")[:60])
	fmt.Println()

	client := &http.Client{Timeout: 30 * time.Second}
	found := 0

	for _, name := range commonNames {
		url := fmt.Sprintf("%s/api/getTestCase", convexURL)
		payload := map[string]string{"name": name}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+secretKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode == 200 {
			// Read the response to get task count
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			var tasks []interface{}
			if err := json.Unmarshal(bodyBytes, &tasks); err == nil {
				fmt.Printf("✓ %s (%d tasks)\n", name, len(tasks))
				found++
			}
		} else {
			resp.Body.Close()
		}
	}

	fmt.Println()
	if found == 0 {
		fmt.Println("No test cases found with common names.")
		fmt.Println()
		fmt.Println("To see all available test cases:")
		fmt.Println("1. Visit the evaluation-platform web UI")
		fmt.Println("2. Navigate to the Test Cases page")
		fmt.Println("3. Or ask an admin to add a /api/listTestCases endpoint")
	} else {
		fmt.Printf("Found %d test case(s).\n", found)
	}
}
