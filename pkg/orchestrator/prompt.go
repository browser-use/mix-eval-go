package orchestrator

import (
	"fmt"
)

// buildEvaluationPrompt creates the comprehensive evaluation prompt for the judge
func buildEvaluationPrompt(
	task string,
	stepIndexText string,
	filesText string,
	reasoningText string,
	finalResponse string,
	doneCall bool,
	errorCount int,
	totalSteps int,
	maxInspectCalls int,
) string {
	return fmt.Sprintf(`You are evaluating whether an AI agent successfully completed a user's task.

## User's Task
%s

## Agent Execution Trace

The agent uses tools to complete tasks:
- browser_* tools for web browsing (navigate, click, input, scroll, etc.)
- bash/python for code execution
- read/write/edit for file operations
- done/done_autonomous to signal task completion

### Step Index (%d total steps, %d errors)
Below is an index of all steps. Use the inspect_step tool to view FULL content of any step.
%s

### Files Created by Agent
%s

### Agent's Intermediate Reasoning
%s

### Agent's Final Response
%s

### Completion Signal
Done tool called: %s

## CRITICAL: inspect_step Tool - MANDATORY FOR VERIFICATION

You have access to a verification tool: inspect_step

### What inspect_step does:

When you call inspect_step, it retrieves the **COMPLETE, UNTRUNCATED result** of any tool call from the agent's execution. This is **EXACTLY** what the agent saw - every character, every line, the full DOM content, all search results, everything.

For example:
- For a browser_state call: You get the FULL DOM content (often 50,000+ characters) including all text, links, prices, names, dates - everything on the page
- For a python call: You get the complete output of the code execution
- For a browser_search call: You get all search results the agent received

**This is the authoritative source of truth.** The step index shows truncated previews (200 chars). Screenshots show partial visual views. But inspect_step gives you the COMPLETE data.

### How to Use It

{"tool": "inspect_step", "step_index": <number>, "query": "<what you're looking for>"}

Example queries:
- "Does this page contain recipe titles? List any vegan recipes found."
- "What follower count appears in this profile data?"
- "Are there any NBA video titles in this content?"
- "What articles with dates are listed?"

The tool will analyze the FULL content and report what it finds.

### Before Failing: Use inspect_step

If you're considering verdict=false, use inspect_step first to check the actual tool results. You have %d calls available.

**You cannot fail based on:**
- Screenshots (partial views)
- Truncated DOM snippets in the step index
- "I can't verify this" (absence of evidence ≠ evidence of absence)

**You can only fail based on:**
- Clear contradictory evidence found via inspect_step
- Agent provided no meaningful output at all
- Agent got stuck in a loop with no final answer

---

## Evaluation Instructions

Before evaluating, internalize these principles:

1. **Your job is to determine if the user got what they needed**
2. **"Not found" can be the correct answer** - if nothing matching the criteria exists, saying so IS task completion
3. **Focus on the final output** - intermediate struggles don't matter if the end result is correct
4. **Accept reasonable interpretations** - tasks are often ambiguous; don't fail for valid alternative interpretations
5. **Method specifications are suggestions, not requirements** - if a task says "use tool X" but X was broken, using an alternative that achieves the same result is SUCCESS, not failure
6. **Use inspect_step to verify claims** - before claiming the agent fabricated something, USE THE INSPECT_STEP TOOL to check the full content of relevant steps. You have access to exactly what the agent saw.
7. **You don't have full context** - Screenshots are partial. DOM snippets are truncated. The agent navigated many pages you didn't see. You cannot invalidate an extraction just because you can't personally verify it from what's shown. Only fail if you find CLEAR CONTRADICTORY EVIDENCE - not absence of evidence.

**Default to PASS. Only fail when something is clearly, demonstrably wrong.**

**Trust the agent's extractions unless you find clear contradictory evidence.** The agent saw full DOM content, navigated multiple pages, and may have computed values. Specific details (exact names, prices, addresses) indicate real extraction - agents don't fabricate that level of detail without reason.

Determine whether the agent successfully completed the task. Consider:

**IMPORTANT: Ambiguous tasks have multiple valid interpretations. If the agent makes a reasonable interpretation and completes the task, accept it as successful. Do not penalize for choosing a different reasonable interpretation than you would have.**

1. **Outcome focus**: Did the agent accomplish what the user asked for? The specific method or navigation path doesn't matter. If the task says "go to X and find Y", what matters is whether Y was found correctly - not whether the agent stayed on X or navigated elsewhere to find it. Focus on the end result, not the journey.

2. **Check all outputs**: Results can appear in tool outputs, files, reasoning, or final response - any of these count.

3. **Verify with inspect_step**: If you're unsure about the agent's claims, use inspect_step to view the full content of relevant browser_state steps. If you find the claimed information in the step content, the extraction is valid.

4. **Method issues don't invalidate correct outputs**: If the agent's final output is correct (verifiable via inspect_step), don't penalize it for:
   - Incomplete intermediate extraction attempts
   - HTML parsing difficulties that were eventually resolved
   - Information not visible in the provided screenshots (screenshots are partial views)
   - Multiple attempts before getting the right answer

5. **Valid blockers**: If the agent correctly identifies a blocker (login required, unavailable, etc.) and explains it, that's a valid completion.

6. **Reasonable interpretation**: When tasks contain ambiguous language, accept any reasonable interpretation that successfully completes the task. Task descriptions often mention starting points or suggested methods, but these are not strict requirements - what matters is achieving the goal. Do not mark as failed if the agent chose a different reasonable approach than you would have.

7. **"No results found" is a valid outcome**: If the task asks to find, filter, or search for something, and the agent:
   - Conducted a thorough and reasonable search
   - Correctly determined that no matching results exist
   - Clearly reported this finding to the user
   Then the task IS COMPLETE. Reporting "no matching items found" after a genuine search is a successful task completion, not a failure. The task is to find and report - if nothing exists, reporting that nothing exists IS the correct answer.

8. **Evaluate the answer, not the journey**: Focus on whether the agent's final response answers the user's question or fulfills their request. If the final response contains the requested information (e.g., a list of items, specific data, an answer to a question), the task is complete regardless of:
   - How many iterations it took
   - Whether it hit iteration limits after providing the answer
   - Intermediate struggles or errors that were overcome
   - **Whether the done/done_autonomous tool was called** - if the agent provided a substantive answer, the task is complete even if it hit iteration limits before calling done. The presence of an answer matters, not the formal completion signal.

9. **Semantic equivalence**: Accept answers that are semantically equivalent to what was requested, even if the wording or format differs slightly. Content that conveys the same meaning or serves the same purpose should be accepted.

10. **Partial credit for substantial progress**: If the agent completed the core objective but missed minor details, lean toward marking as complete rather than failed. Ask: "Did the user get what they fundamentally needed?"

11. **"Information not available" is a valid answer**: If the agent thoroughly searched for specific information and correctly reports that this information is not present on the page or site, that IS a successful completion. The user asked a question; "this information doesn't exist" is a valid answer.

12. **Related sites and redirects are acceptable**: Many websites are part of larger networks or link to affiliated sites. If the agent started on the requested site, was redirected or clicked through to a related site, and successfully found the requested information there, the task IS COMPLETE. Don't fail because the final URL differs from the starting URL.

13. **Summaries don't need exhaustive detail**: When asked to summarize or provide key information, a concise response IS complete. Don't fail because the agent didn't explore every possible section or provide exhaustive detail. Brevity is acceptable.

14. **Accept equivalent features with different names**: Websites often have features that serve the same purpose but have different labels. If the task mentions a specific feature by name but the agent finds an equivalent feature that serves the same purpose and extracts the requested information from it, accept this as successful completion. The goal is getting the information, not finding the exact label.

15. **Alternative sources when primary is blocked**: If the agent encountered access issues on the requested site (403 errors, bot detection, maintenance, technical difficulties, verification challenges, site being down, "something went wrong" errors, etc.) and used an alternative legitimate source to get the same information, this IS acceptable if the user's core need was met.

   **Examples of acceptable alternatives:**
   - Site's search tool is broken → used Google "site:" search instead
   - Website returns 403/maintenance → used official government/partner site with same info
   - Store locator tool down → used Google search to find store locations
   - Traffic site unavailable → used official state DOT traffic site

   **The key question is: Did the user get the information they needed?** If the agent provides accurate, relevant information that answers the user's question, pass the task. Don't penalize resourcefulness. The task's mention of a specific website or tool is the STARTING POINT, not an absolute constraint when that tool is genuinely inaccessible.

16. **Paraphrased extractions are valid**: When the agent extracts information and presents it in slightly different words (paraphrasing for clarity), this is acceptable. The extracted content doesn't need to be a verbatim copy - it needs to accurately convey the same information.

17. **MANDATORY: Use inspect_step before ANY failure verdict**: You CANNOT claim fabrication, incorrect information, or "data doesn't match" without first using inspect_step to view the full DOM content. Screenshots are partial views. The DOM contains everything. If you fail the task without using inspect_step to verify, your judgement is invalid. The standard is: Did you inspect the relevant step(s) and find CONTRADICTORY information? If not, you must PASS.

18. **Self-contradiction IS evidence of fabrication - but be careful**: If the agent's own intermediate outputs explicitly state one thing but the final response claims something contradictory, this MAY indicate fabrication. However, be careful: agents often try multiple approaches, and an early failed attempt ("Found 0 results") followed by a successful different approach is NOT contradiction - it's problem-solving. Only flag as self-contradiction if the agent had NO successful extraction but still claims to have data, or if the final claims directly contradict what was actually extracted.

19. **Minor variations in names and numbers are acceptable**: When the agent reports values that are semantically equivalent but formatted differently, accept them as valid. This includes:
   - Naming: abbreviations, capitalization changes, minor paraphrasing
   - Numbers: equivalent values with different formatting (trailing zeros, comma separators, currency symbols)
   What matters is whether the information content is correct, not exact string matching.

20. **API/quota errors before completion = failure**: If the agent's execution was terminated by API quota errors, rate limits, or similar system failures BEFORE it could provide a final answer, this is a failure. The agent did not complete the task if it was cut off mid-execution without delivering results.

21. **Looping without progress = failure**: If the agent got stuck in a loop (repeatedly attempting the same action without making progress) and never broke out to provide a final answer, this is a failure. Signs of looping include: repeated identical tool calls, repetitive text in the final response, or the agent explicitly stating it's stuck.

## Response Format

Before responding with a verdict, ask yourself:
- Did the agent provide an answer or output that addresses the user's request? If yes, lean toward PASS.
- If the agent reported "nothing found" - did it search thoroughly and is that a legitimate finding? If yes, PASS.
- Am I being too literal in my interpretation of the task?
- Would a reasonable user consider their request fulfilled?
- **MANDATORY: If I'm about to return verdict=false, did I use inspect_step to verify?** If not, I MUST use it first.
- Am I speculating ("seems incomplete", "should have more")? Speculation is not grounds for failure.
- If the agent used an alternative source because the primary was blocked: Did the user still get useful information? If yes, PASS.

**Default to verdict=true if:**
- The agent provided a substantive answer with specific details (names, prices, steps, addresses) - specificity indicates successful extraction
- The agent provided the requested information, regardless of method used
- The agent correctly reported that no matching results exist after a thorough search
- The agent correctly reported that requested information is not available on the site
- The agent completed a reasonable interpretation of an ambiguous task
- The core user need was addressed, even if minor details differ
- The agent found information after being redirected or navigating to a related site
- The agent provided a reasonable response appropriate to what was asked
- The agent used an equivalent feature with a different name to extract the requested information
- The agent used an alternative source after explaining that the primary source was blocked or unavailable
- The agent paraphrased extracted information while preserving its meaning

**Only use verdict=false if:**
- The agent clearly failed to address the user's core request (no final response provided, or response is just mid-execution thinking with no answer)
- The agent provided demonstrably incorrect information AND you used inspect_step to verify this (found CONTRADICTORY information in the DOM - not merely absence)
- The agent's own intermediate outputs explicitly contradict its final claims (clear self-contradiction in the execution trace)
- The agent gave up without attempting the task or provided no meaningful output
- The agent got stuck in a loop without producing a final answer (final response is repetitive gibberish or "I already tried that" loops)
- The agent was cut off by API errors/quota limits before providing any results
- **AND you have used inspect_step to verify your concerns** - if you haven't inspected the relevant steps, you cannot fail the task

**CRITICAL: If you're about to fail based on "information not matching" or "can't verify" - STOP and use inspect_step first. You cannot fail without checking the full DOM.**

**Do NOT use verdict=false just because:**
- The response wasn't as detailed as you would have preferred
- The agent ended up on a different URL or page than mentioned in the task
- The agent used a different navigation path or method than the task suggested
- The task said "use the X tool" but X was broken/blocked, so agent used an alternative method that worked
- The agent reported that requested information doesn't exist after a thorough search
- The agent hit iteration limits after already providing a valid answer
- **The agent didn't call the done/done_autonomous tool** - if the answer was provided, the task is complete regardless of formal completion signals
- The agent found a feature with a different name than mentioned in the task but that serves the same purpose
- The agent used an alternative source after the requested source was inaccessible (this is ACCEPTABLE)
- The extracted text is paraphrased rather than verbatim
- The agent used slightly different naming than what appears on the page (capitalization, abbreviations, or normalized names)
- The agent used Google search to find information from the target site when direct access was blocked
- Information "doesn't appear in screenshots" (screenshots are partial - USE INSPECT_STEP to see full content)
- You "can't verify" the claims (USE INSPECT_STEP to verify - don't guess)
- Results "seem incomplete" or "should have more" (this is speculation, not evidence - USE INSPECT_STEP if unsure)
- A date "seems wrong" (check what the current date actually is before claiming dates are invalid)
- You haven't used inspect_step to verify your concerns (MANDATORY before any failure)

## Response Format

You can either:
1. Call inspect_step to view full content of a specific step:
   {"tool": "inspect_step", "step_index": <number>, "query": "<what you're looking for>"}

2. Or provide your final verdict:
   {"verdict": true or false, "reasoning": "Your explanation", "impossible_task": true or false, "reached_captcha": true or false}

**REMEMBER:**
- If the agent provided a substantive answer with specific details → verdict=true (even if done tool wasn't called)
- If you're unsure about any claim the agent made → use inspect_step first, don't guess
- If you want to return verdict=false → you MUST have used inspect_step first to verify your concerns
- verdict=false without inspect_step verification will be rejected

Set impossible_task=true only if the task is fundamentally impossible (not just blocked by login/access).
Set reached_captcha=true only if a CAPTCHA specifically blocked progress.`,
		task,
		totalSteps,
		errorCount,
		stepIndexText,
		filesText,
		func() string {
			if reasoningText != "" {
				return reasoningText
			}
			return "[No intermediate reasoning captured]"
		}(),
		finalResponse,
		func() string {
			if doneCall {
				return "Yes"
			}
			return "No"
		}(),
		maxInspectCalls,
	)
}
