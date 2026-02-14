#!/usr/bin/env python3
"""Identify Direct Web Scraping tasks that require authentication."""

import json
import re
from urllib.parse import urlparse

# Platforms that typically require authentication
AUTH_REQUIRED_PLATFORMS = {
    'instagram.com': 'Social Media - Login Required',
    'facebook.com': 'Social Media - Login Required',
    'linkedin.com': 'Social Media - Login Required',
    'twitter.com': 'Social Media - Login Required',
    'x.com': 'Social Media - Login Required',
    'upwork.com': 'Job Platform - Login Required',
    'glassdoor.com': 'Job Platform - Login Required',
    'airbnb.ca': 'Booking Platform - Login Often Required',
    'zillow.com': 'Real Estate - Premium Features Need Login',
    'redfin.com': 'Real Estate - Some Features Need Login',
    'redfin.ca': 'Real Estate - Some Features Need Login',
    'sec.gov': 'Government - Some EDGAR filings may need registration',
    'stake.com': 'Betting Platform - Login Required',
    'breaking-bet.com': 'Betting Platform - Login Required',
    'nseindia.com': 'Stock Exchange - Login for detailed data',
    'bseindia.com': 'Stock Exchange - Login for detailed data',
    'idx.co.id': 'Stock Exchange - Login for detailed data',
    'flipkart.com': 'E-commerce - Account for some features',
    'taobao.com': 'E-commerce - Account often required',
    'daraz.pk': 'E-commerce - Account for some features',
    'eprocure.gov.in': 'Government Procurement - Registration Required',
    'philgeps.gov.ph': 'Government Procurement - Registration Required',
    'contractsfinder.service.gov.uk': 'Government - Registration May Be Required',
    'tenderned.nl': 'Tender Platform - May Need Registration',
    'acquistinretepa.it': 'Tender Platform - May Need Registration',
    'remote.team': 'Job Platform - Login Required',
    'apna.co': 'Job Platform - Login Required',
}

# Keywords that indicate authentication might be required
AUTH_KEYWORDS = [
    'login', 'sign in', 'signin', 'log in',
    'credentials', 'username', 'password',
    'authentication', 'auth',
    'account', 'register', 'registration',
    'oauth', 'social login',
    'guest checkout',
    'cookie', 'session',
    'paywall', 'subscription',
    'member', 'membership',
    'authenticated', 'authorize',
]

# Explicit phrases that indicate no auth
NO_AUTH_PHRASES = [
    'do not try to login',
    'you do not have credentials',
    'no credentials',
    'without login',
    'guest checkout',
]

def extract_urls(text):
    """Extract URLs from text."""
    url_pattern = r'https?://[^\s\'"<>\)\],]+'
    urls = re.findall(url_pattern, text)

    www_pattern = r'www\.[^\s\'"<>\)\],]+'
    www_urls = re.findall(www_pattern, text)

    domain_pattern = r'\b([a-zA-Z0-9-]+\.(?:com|org|net|edu|gov|io|co|ai|app|dev|tech|info|biz|us|uk|ca|au|in|ph|pk|be|se|il|vn|mx|za|de|jp|nl|it|ch|global|video|so))\b'
    domains = re.findall(domain_pattern, text)

    all_urls = urls + ['https://' + url for url in www_urls] + ['https://' + d for d in domains]

    return all_urls

def extract_domain(url):
    """Extract clean domain from URL."""
    try:
        url = url.rstrip('.,;:')
        parsed = urlparse(url)
        domain = parsed.netloc or parsed.path.split('/')[0]
        domain = domain.replace('www.', '')
        return domain.lower()
    except:
        return None

def check_auth_required(task):
    """Check if task requires authentication."""
    task_text = task.get('confirmed_task', '').lower()
    task_id = task.get('task_id', 'unknown')

    # Check for explicit "no auth" instructions
    for phrase in NO_AUTH_PHRASES:
        if phrase in task_text:
            return {
                'auth_required': 'EXPLICITLY_NO',
                'reason': f'Task explicitly states: "{phrase}"',
                'confidence': 'HIGH',
                'task_id': task_id,
                'task_snippet': task.get('confirmed_task', '')[:200]
            }

    # Extract URLs and check against known auth-required platforms
    urls = extract_urls(task.get('confirmed_task', ''))
    auth_platforms = []

    for url in urls:
        domain = extract_domain(url)
        if domain in AUTH_REQUIRED_PLATFORMS:
            auth_platforms.append({
                'domain': domain,
                'reason': AUTH_REQUIRED_PLATFORMS[domain],
                'url': url
            })

    # Check for auth-related keywords
    auth_keywords_found = []
    for keyword in AUTH_KEYWORDS:
        if keyword in task_text:
            auth_keywords_found.append(keyword)

    # Determine auth requirement
    if auth_platforms:
        return {
            'auth_required': 'YES',
            'reason': 'Known authentication-required platform',
            'platforms': auth_platforms,
            'keywords': auth_keywords_found,
            'confidence': 'HIGH',
            'task_id': task_id,
            'task_snippet': task.get('confirmed_task', '')[:200]
        }
    elif len(auth_keywords_found) >= 2:
        return {
            'auth_required': 'LIKELY',
            'reason': 'Multiple authentication-related keywords found',
            'keywords': auth_keywords_found,
            'confidence': 'MEDIUM',
            'task_id': task_id,
            'task_snippet': task.get('confirmed_task', '')[:200]
        }
    elif auth_keywords_found:
        return {
            'auth_required': 'POSSIBLE',
            'reason': 'Some authentication-related keywords found',
            'keywords': auth_keywords_found,
            'confidence': 'LOW',
            'task_id': task_id,
            'task_snippet': task.get('confirmed_task', '')[:200]
        }

    return None

def main():
    input_file = 'data/runs/PostHog Cleaned Feb 2026 (1).json'
    output_file = 'results/auth_detection/auth_required_tasks.txt'
    detailed_output_file = 'results/auth_detection/auth_required_tasks_detailed.json'

    # Read the JSON file
    with open(input_file, 'r') as f:
        tasks = json.load(f)

    # Filter for Direct Web Scraping tasks
    scraping_tasks = [task for task in tasks if task.get('category') == 'Direct Web Scraping']

    print(f"Total Direct Web Scraping tasks: {len(scraping_tasks)}")

    # Categorize tasks by auth requirement
    auth_required_yes = []
    auth_required_likely = []
    auth_required_possible = []
    auth_explicitly_no = []
    no_auth_detected = []

    for task in scraping_tasks:
        result = check_auth_required(task)

        if result:
            if result['auth_required'] == 'YES':
                auth_required_yes.append(result)
            elif result['auth_required'] == 'LIKELY':
                auth_required_likely.append(result)
            elif result['auth_required'] == 'POSSIBLE':
                auth_required_possible.append(result)
            elif result['auth_required'] == 'EXPLICITLY_NO':
                auth_explicitly_no.append(result)
        else:
            no_auth_detected.append({
                'task_id': task.get('task_id'),
                'task_snippet': task.get('confirmed_task', '')[:200]
            })

    # Write summary to text file
    with open(output_file, 'w') as f:
        f.write("Authentication Requirements for Direct Web Scraping Tasks\n")
        f.write("=" * 80 + "\n\n")

        f.write(f"Total Direct Web Scraping Tasks: {len(scraping_tasks)}\n\n")

        f.write(f"Authentication Required (HIGH confidence): {len(auth_required_yes)}\n")
        f.write(f"Authentication Likely (MEDIUM confidence): {len(auth_required_likely)}\n")
        f.write(f"Authentication Possible (LOW confidence): {len(auth_required_possible)}\n")
        f.write(f"Authentication Explicitly NOT Required: {len(auth_explicitly_no)}\n")
        f.write(f"No Auth Indicators Detected: {len(no_auth_detected)}\n")

        f.write("\n" + "=" * 80 + "\n\n")

        # YES - Authentication Required
        f.write(f"1. AUTHENTICATION REQUIRED (HIGH CONFIDENCE) - {len(auth_required_yes)} tasks\n")
        f.write("-" * 80 + "\n")
        for i, result in enumerate(auth_required_yes, 1):
            f.write(f"\n{i}. Task ID: {result['task_id']}\n")
            f.write(f"   Reason: {result['reason']}\n")
            if 'platforms' in result:
                f.write(f"   Platforms:\n")
                for platform in result['platforms']:
                    f.write(f"     - {platform['domain']}: {platform['reason']}\n")
            if result.get('keywords'):
                f.write(f"   Keywords found: {', '.join(result['keywords'][:5])}\n")
            f.write(f"   Task: {result['task_snippet']}...\n")

        # LIKELY
        f.write("\n" + "=" * 80 + "\n\n")
        f.write(f"2. AUTHENTICATION LIKELY (MEDIUM CONFIDENCE) - {len(auth_required_likely)} tasks\n")
        f.write("-" * 80 + "\n")
        for i, result in enumerate(auth_required_likely, 1):
            f.write(f"\n{i}. Task ID: {result['task_id']}\n")
            f.write(f"   Reason: {result['reason']}\n")
            f.write(f"   Keywords found: {', '.join(result['keywords'])}\n")
            f.write(f"   Task: {result['task_snippet']}...\n")

        # POSSIBLE
        f.write("\n" + "=" * 80 + "\n\n")
        f.write(f"3. AUTHENTICATION POSSIBLE (LOW CONFIDENCE) - {len(auth_required_possible)} tasks\n")
        f.write("-" * 80 + "\n")
        for i, result in enumerate(auth_required_possible, 1):
            f.write(f"\n{i}. Task ID: {result['task_id']}\n")
            f.write(f"   Keywords found: {', '.join(result['keywords'])}\n")
            f.write(f"   Task: {result['task_snippet']}...\n")

        # EXPLICITLY NO
        f.write("\n" + "=" * 80 + "\n\n")
        f.write(f"4. AUTHENTICATION EXPLICITLY NOT REQUIRED - {len(auth_explicitly_no)} tasks\n")
        f.write("-" * 80 + "\n")
        for i, result in enumerate(auth_explicitly_no, 1):
            f.write(f"\n{i}. Task ID: {result['task_id']}\n")
            f.write(f"   Reason: {result['reason']}\n")
            f.write(f"   Task: {result['task_snippet']}...\n")

    # Write detailed JSON
    detailed_output = {
        'summary': {
            'total_scraping_tasks': len(scraping_tasks),
            'auth_required_high': len(auth_required_yes),
            'auth_required_medium': len(auth_required_likely),
            'auth_required_low': len(auth_required_possible),
            'auth_explicitly_no': len(auth_explicitly_no),
            'no_auth_detected': len(no_auth_detected)
        },
        'auth_required_yes': auth_required_yes,
        'auth_required_likely': auth_required_likely,
        'auth_required_possible': auth_required_possible,
        'auth_explicitly_no': auth_explicitly_no,
        'no_auth_detected': no_auth_detected
    }

    with open(detailed_output_file, 'w') as f:
        json.dump(detailed_output, f, indent=2)

    print(f"\n✓ Written summary to: {output_file}")
    print(f"✓ Written detailed data to: {detailed_output_file}")

    print(f"\nSummary:")
    print(f"  Authentication Required (HIGH):   {len(auth_required_yes):3d} tasks")
    print(f"  Authentication Likely (MEDIUM):   {len(auth_required_likely):3d} tasks")
    print(f"  Authentication Possible (LOW):    {len(auth_required_possible):3d} tasks")
    print(f"  Auth Explicitly NOT Required:     {len(auth_explicitly_no):3d} tasks")
    print(f"  No Auth Indicators:               {len(no_auth_detected):3d} tasks")

if __name__ == '__main__':
    main()
