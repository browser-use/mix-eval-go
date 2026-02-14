#!/usr/bin/env python3
"""Analyze how agents bypassed authentication requirements."""

import json
from difflib import SequenceMatcher

def similar(a, b):
    """Calculate similarity ratio between two strings."""
    return SequenceMatcher(None, a.lower(), b.lower()).ratio()

def find_eval_task(task_id, task_snippet, eval_results):
    """Find matching eval result for a task."""
    best_match = None
    best_ratio = 0

    for eval_task in eval_results:
        eval_text = eval_task.get('task', '')
        ratio = similar(task_snippet, eval_text)

        if ratio > best_ratio:
            best_ratio = ratio
            best_match = eval_task

    if best_ratio >= 0.7:
        return best_match

    return None

def main():
    auth_file = 'results/auth_detection/auth_required_tasks_detailed.json'
    eval_file = 'data/runs/Evals Export.json'
    output_file = 'results/bypass_methods/auth_bypass_methods.txt'

    # Load data
    with open(auth_file, 'r') as f:
        auth_data = json.load(f)

    with open(eval_file, 'r') as f:
        eval_results = json.load(f)

    # Focus on HIGH confidence auth-required tasks
    auth_tasks = auth_data.get('auth_required_yes', [])

    with open(output_file, 'w') as f:
        f.write("Authentication Bypass Methods - Direct Web Scraping Tasks\n")
        f.write("=" * 80 + "\n\n")
        f.write("Analysis of how agents attempted to complete auth-required tasks without\n")
        f.write("having actual login credentials.\n\n")
        f.write("=" * 80 + "\n\n")

        bypassed_count = 0
        failed_count = 0
        not_evaluated_count = 0

        for auth_task in auth_tasks:
            task_id = auth_task.get('task_id')
            task_snippet = auth_task.get('task_snippet', '')
            platforms = auth_task.get('platforms', [])

            # Find eval result
            eval_task = find_eval_task(task_id, task_snippet, eval_results)

            if not eval_task:
                not_evaluated_count += 1
                continue

            # Extract key information
            self_report = eval_task.get('selfReportCompleted', False)
            judgement = eval_task.get('OM2W_judgement', '')
            final_response = eval_task.get('finalResultResponse', '')

            # Determine bypass method from judgement and response
            platform_names = ', '.join([p['domain'] for p in platforms]) if platforms else 'Unknown'

            f.write(f"\nTask ID: {task_id}\n")
            f.write(f"Platform: {platform_names}\n")
            f.write(f"Self-reported: {self_report}\n")
            f.write(f"-" * 80 + "\n")

            # Analyze bypass method from judgement
            judgement_lower = judgement.lower()
            response_lower = final_response.lower() if final_response else ''

            bypass_method = "UNKNOWN"
            success = False

            # Pattern 1: Alternative platform
            if any(word in judgement_lower for word in ['instead', 'used', 'primarily', 'rather than']):
                if 'craigslist' in judgement_lower:
                    bypass_method = "PLATFORM SUBSTITUTION - Used Craigslist instead of target platform"
                elif 'trulia' in judgement_lower or 'google' in judgement_lower:
                    bypass_method = "PLATFORM SUBSTITUTION - Used Trulia/Google instead of target platform"
                elif 'data.gov.in' in judgement_lower:
                    bypass_method = "PLATFORM SUBSTITUTION - Used data.gov.in instead of eprocure.gov.in"
                else:
                    bypass_method = "PLATFORM SUBSTITUTION - Used alternative data source"

            # Pattern 2: Third-party service
            elif 'inflact' in judgement_lower or 'downloader' in judgement_lower:
                bypass_method = "THIRD-PARTY SERVICE - Used Inflact.com as intermediary scraper"
                success = True

            # Pattern 3: Public/unauthenticated pages
            elif 'navigated' in judgement_lower and 'extracted' in judgement_lower:
                if any(word in judgement_lower for word in ['public', 'available', 'accessible']):
                    bypass_method = "PUBLIC ACCESS - Data available without authentication"
                    success = True
                else:
                    bypass_method = "DIRECT SCRAPING - Attempted direct access (unclear if auth needed)"
                    success = 'successfully' in judgement_lower or 'produced' in judgement_lower or 'extracted' in judgement_lower

            # Pattern 4: API/structured data
            elif 'api' in judgement_lower or 'json' in judgement_lower:
                bypass_method = "API ACCESS - Used public or unauthenticated API endpoint"
                success = True

            # Check for actual success indicators
            if 'successfully' in judgement_lower or 'extracted' in judgement_lower or 'produced' in judgement_lower:
                if 'not' not in judgement_lower.split('extracted')[0][-20:]:  # Check if "not extracted"
                    success = True

            if 'failed' in judgement_lower or 'did not' in judgement_lower or 'could not' in judgement_lower:
                success = False

            f.write(f"Bypass Method: {bypass_method}\n")
            f.write(f"Success: {'YES' if success else 'NO'}\n\n")

            f.write(f"Task Description:\n{task_snippet[:200]}...\n\n")

            if judgement:
                f.write(f"Judge Analysis:\n{judgement[:500]}...\n\n")

            if final_response:
                # Extract key insights from final response
                response_excerpt = final_response[:400]
                f.write(f"Agent's Approach (excerpt):\n{response_excerpt}...\n\n")

            f.write("=" * 80 + "\n")

            if success:
                bypassed_count += 1
            else:
                failed_count += 1

        # Summary
        f.write(f"\n\nSUMMARY\n")
        f.write("=" * 80 + "\n")
        f.write(f"Total HIGH-confidence auth-required tasks: {len(auth_tasks)}\n")
        f.write(f"Evaluated: {bypassed_count + failed_count}\n")
        f.write(f"Not evaluated: {not_evaluated_count}\n\n")
        f.write(f"Successfully bypassed auth: {bypassed_count}\n")
        f.write(f"Failed to bypass: {failed_count}\n\n")

        f.write("Common Bypass Strategies:\n")
        f.write("1. PLATFORM SUBSTITUTION - Use alternative sites with similar data\n")
        f.write("2. THIRD-PARTY SERVICES - Use scraper/downloader intermediaries\n")
        f.write("3. PUBLIC ACCESS - Target public pages that don't require auth\n")
        f.write("4. API ACCESS - Use public/unauthenticated API endpoints\n")

    print(f"✓ Written analysis to: {output_file}")
    print(f"\nSummary:")
    print(f"  Total auth-required tasks: {len(auth_tasks)}")
    print(f"  Successfully bypassed: {bypassed_count}")
    print(f"  Failed to bypass: {failed_count}")
    print(f"  Not evaluated: {not_evaluated_count}")

if __name__ == '__main__':
    main()
