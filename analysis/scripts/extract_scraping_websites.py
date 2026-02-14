#!/usr/bin/env python3
"""Extract all websites from Direct Web Scraping tasks."""

import json
import re
from urllib.parse import urlparse
from collections import Counter

def extract_urls(text):
    """Extract URLs from text using regex."""
    # Match http/https URLs
    url_pattern = r'https?://[^\s\'"<>\)\],]+'
    urls = re.findall(url_pattern, text)

    # Also look for www. domains without protocol
    www_pattern = r'www\.[^\s\'"<>\)\],]+'
    www_urls = re.findall(www_pattern, text)

    # Also look for domain.com patterns
    domain_pattern = r'\b([a-zA-Z0-9-]+\.(?:com|org|net|edu|gov|io|co|ai|app|dev|tech|info|biz|us|uk|ca|au|in))\b'
    domains = re.findall(domain_pattern, text)

    all_urls = urls + ['https://' + url for url in www_urls] + ['https://' + d for d in domains]

    return all_urls

def extract_domain(url):
    """Extract clean domain from URL."""
    try:
        # Remove trailing punctuation and clean up
        url = url.rstrip('.,;:')
        parsed = urlparse(url)
        domain = parsed.netloc or parsed.path.split('/')[0]
        # Remove www. prefix for consistency
        domain = domain.replace('www.', '')
        return domain.lower()
    except:
        return None

def main():
    input_file = 'data/runs/PostHog Cleaned Feb 2026 (1).json'
    output_file = 'results/websites/direct_web_scraping_websites.txt'
    detailed_output_file = 'results/websites/direct_web_scraping_websites_detailed.json'

    # Read the JSON file
    with open(input_file, 'r') as f:
        tasks = json.load(f)

    # Filter for Direct Web Scraping tasks
    scraping_tasks = [task for task in tasks if task.get('category') == 'Direct Web Scraping']

    print(f"Total Direct Web Scraping tasks: {len(scraping_tasks)}")

    # Extract all URLs and domains
    all_urls = []
    domain_to_tasks = {}

    for task in scraping_tasks:
        task_text = task.get('confirmed_task', '')
        task_id = task.get('task_id', 'unknown')

        urls = extract_urls(task_text)

        for url in urls:
            domain = extract_domain(url)
            if domain and domain:
                all_urls.append(url)
                if domain not in domain_to_tasks:
                    domain_to_tasks[domain] = []
                domain_to_tasks[domain].append({
                    'task_id': task_id,
                    'url': url,
                    'task_snippet': task_text[:100] + '...' if len(task_text) > 100 else task_text
                })

    # Count unique domains
    domain_counter = Counter([extract_domain(url) for url in all_urls if extract_domain(url)])

    print(f"\nTotal URLs found: {len(all_urls)}")
    print(f"Unique domains: {len(domain_counter)}")

    # Write simple list to text file
    with open(output_file, 'w') as f:
        f.write(f"Direct Web Scraping Websites\n")
        f.write(f"=" * 80 + "\n")
        f.write(f"Total tasks: {len(scraping_tasks)}\n")
        f.write(f"Total URLs found: {len(all_urls)}\n")
        f.write(f"Unique domains: {len(domain_counter)}\n")
        f.write(f"\n" + "=" * 80 + "\n\n")

        f.write("DOMAINS SORTED BY FREQUENCY:\n")
        f.write("-" * 80 + "\n")
        for domain, count in domain_counter.most_common():
            f.write(f"{count:3d}x  {domain}\n")

        f.write("\n" + "=" * 80 + "\n\n")
        f.write("ALL UNIQUE DOMAINS (ALPHABETICALLY):\n")
        f.write("-" * 80 + "\n")
        for domain in sorted(domain_counter.keys()):
            f.write(f"{domain}\n")

    # Write detailed JSON with task mappings
    detailed_output = {
        'summary': {
            'total_scraping_tasks': len(scraping_tasks),
            'total_urls_found': len(all_urls),
            'unique_domains': len(domain_counter)
        },
        'domain_frequency': dict(domain_counter.most_common()),
        'domain_to_tasks': domain_to_tasks
    }

    with open(detailed_output_file, 'w') as f:
        json.dump(detailed_output, f, indent=2)

    print(f"\n✓ Written summary to: {output_file}")
    print(f"✓ Written detailed data to: {detailed_output_file}")

    print("\nTop 10 most common domains:")
    for domain, count in domain_counter.most_common(10):
        print(f"  {count:3d}x  {domain}")

if __name__ == '__main__':
    main()
