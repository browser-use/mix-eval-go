#!/usr/bin/env python3
"""Analyze completion rates for authentication-required tasks."""

import json
from difflib import SequenceMatcher

def similar(a, b):
    """Calculate similarity ratio between two strings."""
    return SequenceMatcher(None, a.lower(), b.lower()).ratio()

def find_best_match(task_text, eval_tasks, threshold=0.8):
    """Find the best matching task in eval results."""
    best_match = None
    best_ratio = 0

    for eval_task in eval_tasks:
        eval_text = eval_task.get('task', '')
        ratio = similar(task_text, eval_text)

        if ratio > best_ratio:
            best_ratio = ratio
            best_match = eval_task

    if best_ratio >= threshold:
        return best_match, best_ratio

    return None, best_ratio

def analyze_judgement(judgement_text):
    """Analyze the OM2W judgement to determine if task was successful."""
    if not judgement_text:
        return None

    judgement_lower = judgement_text.lower()

    # Strong failure indicators
    failure_indicators = [
        'not completed',
        'not fulfilled',
        'did not',
        'failed to',
        'was not',
        'were not',
        'cannot',
        'could not',
        'unable to',
        'no evidence',
        'not successful',
    ]

    # Strong success indicators
    success_indicators = [
        'successfully',
        'completed',
        'fulfilled',
        'extracted',
        'provided',
        'accomplished',
    ]

    failure_count = sum(1 for indicator in failure_indicators if indicator in judgement_lower)
    success_count = sum(1 for indicator in success_indicators if indicator in judgement_lower)

    if failure_count > success_count:
        return 'FAIL'
    elif success_count > failure_count:
        return 'PASS'
    else:
        return 'UNCLEAR'

def main():
    auth_file = 'results/auth_detection/auth_required_tasks_detailed.json'
    eval_file = 'data/runs/Evals Export.json'
    output_file = 'results/completion_analysis/auth_task_completion_analysis.txt'
    detailed_output_file = 'results/completion_analysis/auth_task_completion_detailed.json'

    # Load auth-required tasks
    with open(auth_file, 'r') as f:
        auth_data = json.load(f)

    # Load eval results
    with open(eval_file, 'r') as f:
        eval_results = json.load(f)

    print(f"Total eval results: {len(eval_results)}")

    # Analyze each confidence level
    results_by_confidence = {}

    for confidence_level in ['auth_required_yes', 'auth_required_likely', 'auth_required_possible']:
        auth_tasks = auth_data.get(confidence_level, [])

        matched_tasks = []
        unmatched_tasks = []

        for auth_task in auth_tasks:
            task_id = auth_task.get('task_id')
            task_snippet = auth_task.get('task_snippet', '')

            # Try to find matching eval result
            best_match, ratio = find_best_match(task_snippet, eval_results, threshold=0.7)

            if best_match:
                self_report = best_match.get('selfReportCompleted', False)
                judgement = best_match.get('OM2W_judgement', '')
                final_response = best_match.get('finalResultResponse', '')

                judgement_result = analyze_judgement(judgement)

                matched_tasks.append({
                    'task_id': task_id,
                    'task_snippet': task_snippet[:150],
                    'match_ratio': ratio,
                    'selfReportCompleted': self_report,
                    'has_judgement': bool(judgement),
                    'judgement_result': judgement_result,
                    'judgement': judgement[:300] if judgement else None,
                    'platforms': auth_task.get('platforms', []),
                })
            else:
                unmatched_tasks.append({
                    'task_id': task_id,
                    'task_snippet': task_snippet[:150],
                    'best_ratio': ratio
                })

        results_by_confidence[confidence_level] = {
            'total': len(auth_tasks),
            'matched': len(matched_tasks),
            'unmatched': len(unmatched_tasks),
            'matched_tasks': matched_tasks,
            'unmatched_tasks': unmatched_tasks
        }

    # Calculate statistics
    with open(output_file, 'w') as f:
        f.write("Authentication-Required Tasks - Completion Analysis\n")
        f.write("=" * 80 + "\n\n")

        total_auth_tasks = 0
        total_matched = 0
        total_self_reported = 0
        total_judged = 0
        total_passed = 0
        total_failed = 0
        total_unclear = 0

        for level_name, level_data in results_by_confidence.items():
            level_label = level_name.replace('auth_required_', '').replace('_', ' ').upper()

            f.write(f"\n{level_label}\n")
            f.write("-" * 80 + "\n")
            f.write(f"Total tasks: {level_data['total']}\n")
            f.write(f"Matched in eval results: {level_data['matched']}\n")
            f.write(f"Not found in eval results: {level_data['unmatched']}\n\n")

            if level_data['matched'] > 0:
                self_reported_count = sum(1 for t in level_data['matched_tasks'] if t['selfReportCompleted'])
                judged_count = sum(1 for t in level_data['matched_tasks'] if t['has_judgement'])
                passed_count = sum(1 for t in level_data['matched_tasks'] if t['judgement_result'] == 'PASS')
                failed_count = sum(1 for t in level_data['matched_tasks'] if t['judgement_result'] == 'FAIL')
                unclear_count = sum(1 for t in level_data['matched_tasks'] if t['judgement_result'] == 'UNCLEAR')

                f.write(f"Self-reported completed: {self_reported_count}/{level_data['matched']} ({self_reported_count/level_data['matched']*100:.1f}%)\n")
                f.write(f"Has judge evaluation: {judged_count}/{level_data['matched']} ({judged_count/level_data['matched']*100:.1f}%)\n")
                f.write(f"Judge: PASS: {passed_count}, FAIL: {failed_count}, UNCLEAR: {unclear_count}\n")

                total_self_reported += self_reported_count
                total_judged += judged_count
                total_passed += passed_count
                total_failed += failed_count
                total_unclear += unclear_count

            total_auth_tasks += level_data['total']
            total_matched += level_data['matched']

            # Show detailed results
            if level_data['matched'] > 0:
                f.write("\nDetailed Results:\n")
                for i, task in enumerate(level_data['matched_tasks'], 1):
                    f.write(f"\n{i}. Task ID: {task['task_id']} (Match: {task['match_ratio']:.2f})\n")
                    f.write(f"   Self-reported: {task['selfReportCompleted']}\n")
                    f.write(f"   Judge result: {task['judgement_result']}\n")
                    if task.get('platforms'):
                        platforms_str = ', '.join([p['domain'] for p in task['platforms']])
                        f.write(f"   Platforms: {platforms_str}\n")
                    if task['judgement']:
                        f.write(f"   Judge: {task['judgement']}...\n")
                    f.write(f"   Task: {task['task_snippet']}...\n")

        # Overall summary
        f.write("\n" + "=" * 80 + "\n")
        f.write("\nOVERALL SUMMARY\n")
        f.write("-" * 80 + "\n")
        f.write(f"Total auth-required tasks (HIGH + MEDIUM + LOW): {total_auth_tasks}\n")
        f.write(f"Found in eval results: {total_matched}/{total_auth_tasks} ({total_matched/total_auth_tasks*100:.1f}%)\n\n")

        if total_matched > 0:
            f.write(f"Self-reported completion rate: {total_self_reported}/{total_matched} ({total_self_reported/total_matched*100:.1f}%)\n")
            f.write(f"Judge evaluation coverage: {total_judged}/{total_matched} ({total_judged/total_matched*100:.1f}%)\n\n")

            f.write(f"Judge Results:\n")
            f.write(f"  PASS:    {total_passed:3d} ({total_passed/total_judged*100:.1f}% of judged)\n")
            f.write(f"  FAIL:    {total_failed:3d} ({total_failed/total_judged*100:.1f}% of judged)\n")
            f.write(f"  UNCLEAR: {total_unclear:3d} ({total_unclear/total_judged*100:.1f}% of judged)\n")

    # Save detailed JSON
    detailed_output = {
        'summary': {
            'total_auth_tasks': total_auth_tasks,
            'total_matched': total_matched,
            'total_self_reported': total_self_reported,
            'total_judged': total_judged,
            'total_passed': total_passed,
            'total_failed': total_failed,
            'total_unclear': total_unclear
        },
        'by_confidence_level': results_by_confidence
    }

    with open(detailed_output_file, 'w') as f:
        json.dump(detailed_output, f, indent=2)

    print(f"\n✓ Written analysis to: {output_file}")
    print(f"✓ Written detailed data to: {detailed_output_file}")

    print(f"\nOverall Summary:")
    print(f"  Total auth tasks: {total_auth_tasks}")
    print(f"  Found in evals: {total_matched} ({total_matched/total_auth_tasks*100:.1f}%)")
    if total_matched > 0:
        print(f"  Self-reported complete: {total_self_reported}/{total_matched} ({total_self_reported/total_matched*100:.1f}%)")
        print(f"  Judge evaluated: {total_judged}/{total_matched} ({total_judged/total_matched*100:.1f}%)")
        if total_judged > 0:
            print(f"  Judge PASS: {total_passed} ({total_passed/total_judged*100:.1f}%)")
            print(f"  Judge FAIL: {total_failed} ({total_failed/total_judged*100:.1f}%)")

if __name__ == '__main__':
    main()
