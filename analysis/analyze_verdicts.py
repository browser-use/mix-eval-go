import json
import re

# Load results
with open('no_auth_eval_results.json', 'r') as f:
    data = json.load(f)

# Better judge verdict classification
def classify_verdict(judge_text):
    if not judge_text:
        return "NO_JUDGE"
    
    judge_lower = judge_text.lower()
    
    # Strong PASS indicators
    pass_phrases = [
        'successfully', 'completed', 'delivered', 'fulfilled',
        'extracted and', 'navigated.*and.*extracted', 'produced.*file',
        'provided.*list', 'created.*containing', 'gathered.*details'
    ]
    
    # Strong FAIL indicators  
    fail_phrases = [
        'did not', 'was not completed', 'failed', 'could not',
        'unable to', 'not successfully', 'no.*extracted',
        'agent.*not.*deliver', 'blocked', 'incorrect'
    ]
    
    # Check for failures first (more specific)
    for phrase in fail_phrases:
        if re.search(phrase, judge_lower):
            return "FAIL"
    
    # Then check for passes
    for phrase in pass_phrases:
        if re.search(phrase, judge_lower):
            return "PASS"
    
    return "UNCLEAR"

# Classify all
results = {'PASS': 0, 'FAIL': 0, 'UNCLEAR': 0, 'NO_JUDGE': 0}
details = {'PASS': [], 'FAIL': [], 'UNCLEAR': []}

for task in data['tasks']:
    verdict = classify_verdict(task.get('judge'))
    results[verdict] += 1
    
    if verdict in details:
        details[verdict].append({
            'task_id': task['task_id'],
            'judge_snippet': task.get('judge', '')[:150]
        })

print("=" * 60)
print("NO-AUTH TASKS EVALUATION RESULTS")
print("=" * 60)
print(f"\nTotal no-auth tasks (183): {data['summary']['no_auth']}")
print(f"No-auth tasks found in evals: {data['summary']['no_auth_evaluated']}")
print(f"No-auth tasks with judge verdict: {data['summary']['no_auth_with_judge']}")
print(f"\n" + "=" * 60)
print("JUDGE VERDICTS FOR NO-AUTH TASKS:")
print("=" * 60)
print(f"PASS:    {results['PASS']:3d}  ({results['PASS']/data['summary']['no_auth_with_judge']*100:.1f}%)")
print(f"FAIL:    {results['FAIL']:3d}  ({results['FAIL']/data['summary']['no_auth_with_judge']*100:.1f}%)")
print(f"UNCLEAR: {results['UNCLEAR']:3d}  ({results['UNCLEAR']/data['summary']['no_auth_with_judge']*100:.1f}%)")
print(f"NO_JUDGE:{results['NO_JUDGE']:3d}\n")

print("\nSample PASS verdicts:")
for i, item in enumerate(details['PASS'][:5], 1):
    print(f"{i}. Task {item['task_id']}: {item['judge_snippet']}...")

print("\nSample FAIL verdicts:")
for i, item in enumerate(details['FAIL'][:5], 1):
    print(f"{i}. Task {item['task_id']}: {item['judge_snippet']}...")

# Save classified results
with open('no_auth_classified.json', 'w') as f:
    json.dump({
        'summary': results,
        'details': details
    }, f, indent=2)

print("\n" + "=" * 60)
print(f"Detailed classification saved to no_auth_classified.json")
print("=" * 60)
