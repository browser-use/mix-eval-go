# Mix Eval Go

Go-based evaluation orchestrator for running Mix Eval tasks using browser automation agents.

## Features

- 🚀 **Fast & Concurrent** - Run multiple evaluation tasks in parallel
- 🔧 **Type-Safe** - Built with Go and the Mix Go SDK for compile-time guarantees
- 🌐 **Cloud Browser Support** - Integrates with Browserbase, Brightdata, Hyperbrowser, and Anchor Browser
- 📊 **Judge Evaluation** - Uses Claude to evaluate task completion
- 📡 **Real-time Streaming** - SSE event streaming for live progress updates
- 🎯 **Production-Ready** - Single binary deployment with no dependencies

## Overview

Mix-Eval-Go orchestrates agent evaluations by:
1. Fetching tasks from Convex evaluation platform
2. Creating browser sessions with cloud providers (optional)
3. Executing tasks via Mix Agent with SSE streaming
4. Collecting tool calls and execution history
5. Evaluating results with Claude judge
6. Submitting results back to Convex

## Quick Start

### Prerequisites

1. **Mix Agent** running locally or remotely
   - Default: `http://localhost:8088`
   - See [Mix Agent Setup](https://github.com/recreate-run/mix)

2. **Convex Database** with evaluation API
   - Convex deployment URL
   - API secret key

3. **(Optional) Cloud Browser Providers**
   - Browserbase, Brightdata, Hyperbrowser, or Anchor Browser API keys

### Installation

```bash
# Install dependencies
task install

# Build
task build
```

### Configuration

Copy and configure environment variables:

```bash
cp .env.example .env
# Edit .env with your credentials
```

Required:
- `CONVEX_URL` - Your Convex deployment URL
- `CONVEX_SECRET_KEY` - Convex API secret key

Optional:
- `MIX_AGENT_URL` - Mix Agent URL (default: `http://localhost:8088`)
- `BROWSERBASE_API_KEY`, `BRIGHTDATA_USER`, etc. - Cloud browser provider credentials

### Running Evaluations

```bash
# Load environment variables
source .env

# Run evaluation
./bin/mix-eval-go \
  --test-case "your_test_case_name" \
  --run-id "unique_run_id" \
  --parallel 3
```

## Development

### Development Commands

```bash
task dev                 # Development hot reload
task build               # Build the binary
task test                # Run tests with race detection
task test-e2e            # Run end-to-end tests (requires Mix Agent)
task test-all            # Run all tests including e2e
task tail-dev-log        # View dev server logs
task clean               # Clean build artifacts
task lint                # Run linters
task fmt                 # Format code
task --list-all          # Show all available tasks
```

### Testing

**Unit Tests** (no external dependencies):
```bash
task test
```

**End-to-End Tests** (requires Mix Agent running):
```bash
# 1. Start Mix Agent server in separate terminal
cd ../mix/mix_agent
mix --http-port 8088

# 2. Run E2E tests
task test-e2e
```

**End-to-End Test Coverage**:

1. **Simple Task Test** (`TestEndToEndSimpleTask`)
   - Basic math: "What is 2+2?"
   - Verifies SDK integration and streaming
   - Runtime: ~3 seconds

2. **Browser Automation Test** (`TestEndToEndBrowserAutomation`)
   - Sanity check: Simple math problem
   - Wikipedia extraction: Navigate to cats page, extract intro paragraph
   - Keyword verification: Checks for "Felis catus" in extracted content
   - Real browser automation with tool execution
   - Runtime: ~30 seconds

**Test Characteristics**:
- Real browser automation via Mix Agent
- Real HTTP requests to external sites (Wikipedia)
- SSE event streaming with manual HTTP (SDK streaming bug workaround)
- Tool call execution tracking
- Session management lifecycle
- Content extraction and verification
- **Zero mocking** - all real HTTP/SSE communication

E2E tests are marked with `//go:build e2e` tag and skip by default. They verify the complete workflow from session creation through browser automation to content extraction and validation.

### Project Structure

```
mix-eval-go/
├── cmd/
│   └── main.go                 # CLI entry point
├── pkg/
│   ├── orchestrator/           # Task orchestration & coordination
│   │   ├── orchestrator.go     # Main pipeline with SSE streaming
│   │   ├── extractor.go        # History extraction
│   │   └── judge.go            # Claude evaluation
│   ├── convex/                 # Convex database client
│   │   └── client.go
│   └── providers/              # Browser provider implementations
│       └── browsers.go         # Browserbase, Brightdata, etc.
├── test/
│   └── testdata/
└── bin/                        # Built binaries (runtime)
```

## Architecture

### Evaluation Flow

```
1. Fetch tasks from Convex
2. Create browser session (if cloud provider specified)
3. Create Mix Agent session
4. Stream SSE events in background
5. Send task to Mix Agent
6. Collect tool calls and screenshots
7. Extract execution history
8. Judge evaluates completion
9. Upload screenshots to Convex
10. Submit results to Convex
```

### Browser Providers

Supports multiple cloud browser providers:
- **Browserbase** - High-quality managed browsers
- **Brightdata** - Global proxy network with browsers
- **Hyperbrowser** - Stealth browsing capabilities
- **Anchor Browser** - Mobile and desktop with captcha solving

## Relationship to Other Evaluation Repositories

Mix-Eval-Go is part of a unified evaluation ecosystem with multiple specialized runners sharing a common backend.

### Ecosystem Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                  evaluation-platform                        │
│                  (Convex Backend + UI)                      │
│  - Shared test cases database                               │
│  - Results storage                                          │
│  - REST API endpoints                                       │
│  - Screenshot storage                                       │
└────────┬──────────────┬──────────────┬─────────────────────┘
         │              │              │
         │ REST API     │ REST API     │ REST API
         │              │              │
    ┌────┴───┐     ┌────┴───┐     ┌───┴──────┐
    │        │     │        │     │          │
    ▼        ▼     ▼        ▼     ▼          ▼
┌─────────────┐ ┌─────────────┐ ┌──────────────────┐
│ manus-eval  │ │ mix-eval-go │ │ evaluations-     │
│ (Python)    │ │ (Go)        │ │ internal         │
│             │ │             │ │ (Python)         │
│ Targets:    │ │ Targets:    │ │ Targets:         │
│ Manus Agent │ │ Mix Agent   │ │ browser-use      │
│ (tool-based)│ │ (new agent) │ │ (DOM-based)      │
└─────────────┘ └─────────────┘ └──────────────────┘
```

### Repository Roles

**evaluation-platform** - Central hub providing shared infrastructure for all evaluation runners. Built with React + Convex, it stores test cases, manages runs, hosts judge evaluations, and provides REST API endpoints used by all runners.

**manus-eval** (Python) - Evaluates the Manus agent, which uses explicit tool calls (browser_*, bash, python, read, write) for task execution. Tracks detailed tool calls with arguments, results, and sandbox files. Included as a git submodule in the manus-use repository.

**mix-eval-go** (Go, this repository) - Newest evaluation runner built in Go for the Mix Agent. Uses the Mix Go SDK and communicates via HTTP + SSE. Architecturally inspired by manus-eval but rewritten for performance and type safety.

**evaluations-internal** (Python) - Original evaluation framework targeting the browser-use agent with DOM-based automation. Predecessor to both manus-eval and mix-eval-go, maintaining similar structure but focused on traditional browser automation patterns.

### Key Characteristics

All runners share the same evaluation-platform backend, enabling consistent task definitions and result comparison across different agents. Each runner specializes in its target agent's execution model while following similar architectural patterns: fetch tasks, execute with agent, evaluate with Claude judge, submit results.

Mix-Eval-Go distinguishes itself through Go's performance benefits, compile-time type safety, and single-binary deployment, making it ideal for production environments requiring high concurrency and minimal operational overhead.

## Implementation Status

See [IMPLEMENTATION_STATUS.md](./IMPLEMENTATION_STATUS.md) for detailed status.

**Current Status:** Phase 2-4 complete (Go-Eval Orchestrator)
**Known Limitation:** Mix Agent CDP support (Phase 1) not yet implemented - currently uses local browser mode

## Code Quality

- ✅ Follows Go conventions (gofmt, effective Go)
- ✅ Proper error handling with error wrapping
- ✅ Context propagation for cancellation
- ✅ Type-safe SDK integration
- ✅ Concurrent execution with proper synchronization

## Dependencies

- `github.com/recreate-run/mix-go-sdk v0.2.1` - Mix SDK client
- Standard library only (no other dependencies)

## License

Proprietary
