import json

# Load Direct Web Scraping tasks
with open('direct_web_scraping_tasks.json', 'r') as f:
    dws_tasks = json.load(f)

# Load Evals Export
with open('data/runs/Evals Export.json', 'r') as f:
    evals = json.load(f)

# Create mapping of task text to task_id for DWS tasks
task_text_to_id = {}
for task in dws_tasks:
    task_text_to_id[task['confirmed_task'].strip()] = task['task_id']

# Auth-required task IDs from manual analysis
auth_required_ids = {
    # Hard auth walls
    '2118230', '2147069',  # Instagram
    '1100384', '919843', '1310325',  # Upwork/Apna
    '1244960', '1406205',  # LinkedIn
    '872717', '1123218',  # Betting
    '259349', '1012772',  # Gov procurement
    '342762',  # Mergr
    # Subscription services
    '2094857',  # Beauhurst
    '2136340',  # SEMrush
    '1026443',  # SimilarWeb
    '1225015',  # Crunchbase
    '2309290', '2121476',  # Keyword tools
}

# Match evals to DWS tasks
matched_no_auth = []
matched_auth = []
unmatched = []

for eval_entry in evals:
    task_text = eval_entry.get('task', '').strip()
    
    if task_text in task_text_to_id:
        task_id = task_text_to_id[task_text]
        
        # Check if it has judge verdict
        has_judge = 'OM2W_judgement' in eval_entry
        
        # Classify as auth or no-auth
        if task_id in auth_required_ids:
            matched_auth.append({
                'task_id': task_id,
                'has_judge': has_judge,
                'judge': eval_entry.get('OM2W_judgement', '')[:200] if has_judge else None,
                'self_report': eval_entry.get('selfReportCompleted', False)
            })
        else:
            matched_no_auth.append({
                'task_id': task_id,
                'has_judge': has_judge,
                'judge': eval_entry.get('OM2W_judgement', '')[:200] if has_judge else None,
                'self_report': eval_entry.get('selfReportCompleted', False)
            })

print(f"Total DWS tasks: {len(dws_tasks)}")
print(f"Total evals: {len(evals)}")
print(f"\nAuth-required tasks (18-22): {len(auth_required_ids)}")
print(f"No-auth tasks (179-183): {len(dws_tasks) - len(auth_required_ids)}")
print(f"\nMatched no-auth tasks in evals: {len(matched_no_auth)}")
print(f"Matched auth tasks in evals: {len(matched_auth)}")

# Count judge verdicts for no-auth tasks
no_auth_with_judge = [t for t in matched_no_auth if t['has_judge']]
print(f"\nNo-auth tasks with judge verdict: {len(no_auth_with_judge)}")

# Analyze judge verdicts - simple keyword check for PASS/FAIL
pass_count = 0
fail_count = 0
unclear_count = 0

for task in no_auth_with_judge:
    judge_text = task['judge'].lower() if task['judge'] else ''
    
    # Simple heuristics based on patterns seen
    if 'completed' in judge_text or 'successfully' in judge_text or 'extracted' in judge_text:
        if 'not completed' not in judge_text and 'was not completed' not in judge_text:
            pass_count += 1
            continue
    
    if 'not completed' in judge_text or 'was not completed' in judge_text or 'failed' in judge_text:
        fail_count += 1
        continue
    
    unclear_count += 1

print(f"\nJudge verdicts for no-auth tasks:")
print(f"  PASS (estimated): {pass_count}")
print(f"  FAIL (estimated): {fail_count}")
print(f"  UNCLEAR/OTHER: {unclear_count}")

# Save detailed results
with open('no_auth_eval_results.json', 'w') as f:
    json.dump({
        'summary': {
            'total_dws': len(dws_tasks),
            'auth_required': len(auth_required_ids),
            'no_auth': len(dws_tasks) - len(auth_required_ids),
            'no_auth_evaluated': len(matched_no_auth),
            'no_auth_with_judge': len(no_auth_with_judge),
            'pass_estimated': pass_count,
            'fail_estimated': fail_count,
            'unclear': unclear_count
        },
        'tasks': matched_no_auth
    }, f, indent=2)

print(f"\nDetailed results saved to no_auth_eval_results.json")
