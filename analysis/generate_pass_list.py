import json

# Load data
with open('no_auth_pass_full_list.json', 'r') as f:
    pass_tasks = json.load(f)

# Generate markdown report
md = """# No-Auth Tasks That Clearly Passed (62 tasks)

Based on judge evaluations from `analysis/data/runs/Evals Export.json`

**Summary:** These 62 tasks (35.2% of 176 evaluated no-auth tasks) were classified as PASS based on judge verdicts containing indicators like "successfully", "completed", "delivered", "extracted", etc.

---

"""

for i, task in enumerate(pass_tasks, 1):
    md += f"## {i}. Task ID: {task['task_id']}\n\n"
    md += f"**Task:**\n```\n{task['full_task']}\n```\n\n"
    md += f"**Judge Verdict:**\n```\n{task['judge']}\n```\n\n"
    md += "---\n\n"

# Save markdown
with open('/Users/sarathmenon/Documents/startup/image_generation/browser-use-trial/mix-eval-go/analysis/reports/NO_AUTH_PASS_LIST.md', 'w') as f:
    f.write(md)

# Also create a simple CSV for quick reference
import csv

with open('no_auth_pass_summary.csv', 'w', newline='') as f:
    writer = csv.writer(f)
    writer.writerow(['Task_ID', 'Task_Summary', 'Judge_Verdict_Summary'])
    
    for task in pass_tasks:
        task_summary = task['full_task'][:100].replace('\n', ' ')
        judge_summary = task['judge'][:150].replace('\n', ' ')
        writer.writerow([task['task_id'], task_summary, judge_summary])

print("Generated files:")
print("1. NO_AUTH_PASS_LIST.md - Full details")
print("2. no_auth_pass_summary.csv - Quick reference")
print(f"\nTotal: {len(pass_tasks)} tasks passed")

# Print task IDs for quick reference
print("\nTask IDs (sorted):")
for i in range(0, len(pass_tasks), 10):
    ids = [t['task_id'] for t in pass_tasks[i:i+10]]
    print(", ".join(ids))
