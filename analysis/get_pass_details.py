import json

# Load all data
with open('direct_web_scraping_tasks.json', 'r') as f:
    dws_tasks = json.load(f)

with open('no_auth_classified.json', 'r') as f:
    classified = json.load(f)

with open('data/runs/Evals Export.json', 'r') as f:
    evals = json.load(f)

# Create lookup dicts
task_id_to_task = {t['task_id']: t for t in dws_tasks}
task_text_to_eval = {e['task'].strip(): e for e in evals}

# Get PASS task IDs
pass_ids = [item['task_id'] for item in classified['details']['PASS']]

# Build full details
pass_tasks = []
for task_id in sorted(pass_ids, key=lambda x: int(x)):
    if task_id in task_id_to_task:
        task = task_id_to_task[task_id]
        task_text = task['confirmed_task'].strip()
        
        # Get eval entry
        eval_entry = task_text_to_eval.get(task_text, {})
        judge = eval_entry.get('OM2W_judgement', '')
        
        pass_tasks.append({
            'task_id': task_id,
            'task': task_text[:200] + '...' if len(task_text) > 200 else task_text,
            'full_task': task_text,
            'judge': judge
        })

# Print summary
print(f"{'='*80}")
print(f"NO-AUTH TASKS THAT CLEARLY PASSED (62 tasks)")
print(f"{'='*80}\n")

for i, t in enumerate(pass_tasks, 1):
    print(f"{i}. Task ID: {t['task_id']}")
    print(f"   Task: {t['task']}")
    print(f"   Judge: {t['judge'][:300]}...")
    print()

# Save full list
with open('no_auth_pass_full_list.json', 'w') as f:
    json.dump(pass_tasks, f, indent=2)

print(f"\n{'='*80}")
print(f"Full details saved to: no_auth_pass_full_list.json")
print(f"{'='*80}")
