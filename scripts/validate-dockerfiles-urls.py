#!/usr/bin/env python3
"""
Validate that download URLs in generated Dockerfiles exist on download.jetbrains.com

Usage:
    python validate_downloads.py
"""
import os
import re
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import List, Tuple
from urllib.request import Request, urlopen
from urllib.error import HTTPError, URLError

DOCKERFILES_DIR = "dockerfiles"
TIMEOUT = 10
MAX_WORKERS = 10

def extract_urls_from_dockerfile(dockerfile_path: str) -> List[str]:
    url_pattern = re.compile(r'ARG QD_(?:DOWNLOAD|CHECKSUM)_URL.*?="(https://[^"]+)"')
    urls = []
    try:
        with open(dockerfile_path, "r", encoding="utf-8") as f:
            content = f.read()
            urls = url_pattern.findall(content)
    except OSError as e:
        print(f"Warning: Could not read {dockerfile_path}: {e}")
    return urls

def extract_all_urls(dockerfiles_dir: str) -> List[str]:
    all_urls = set()
    if not os.path.isdir(dockerfiles_dir):
        print(f"Error: Directory '{dockerfiles_dir}' not found.")
        sys.exit(1)
    for variant in os.listdir(dockerfiles_dir):
        dockerfile_path = os.path.join(dockerfiles_dir, variant, "Dockerfile")
        if os.path.isfile(dockerfile_path):
            urls = extract_urls_from_dockerfile(dockerfile_path)
            all_urls.update(urls)
    return sorted(list(all_urls))

def validate_url(url: str, timeout: int) -> Tuple[str, bool, str]:
    try:
        req = Request(url, method="HEAD")
        with urlopen(req, timeout=timeout) as response:
            if response.status == 200:
                return (url, True, "")
            else:
                return (url, False, f"HTTP {response.status}")
    except HTTPError as e:
        return (url, False, f"HTTP {e.code}")
    except URLError as e:
        return (url, False, f"URL Error: {e.reason}")
    except Exception as e:
        return (url, False, str(e))

def validate_urls_parallel(urls: List[str]) -> List[Tuple[str, bool, str]]:
    results = []
    with ThreadPoolExecutor(max_workers=MAX_WORKERS) as executor:
        future_to_url = {executor.submit(validate_url, url, TIMEOUT): url for url in urls}
        for future in as_completed(future_to_url):
            result = future.result()
            results.append(result)
            if result[1]:
                print(f"✓ {result[0]}")
            else:
                print(f"✗ {result[0]} - {result[2]}")
    return results

def main() -> None:
    print(f"Extracting URLs from Dockerfiles in '{DOCKERFILES_DIR}'...")
    urls = extract_all_urls(DOCKERFILES_DIR)
    if not urls:
        print("No URLs found in Dockerfiles.")
        sys.exit(0)
    print(f"Found {len(urls)} unique URLs. Validating in parallel (timeout: {TIMEOUT}s)...")
    print()
    results = validate_urls_parallel(urls)
    print()
    print("=" * 80)
    valid_count = sum(1 for _, is_valid, _ in results if is_valid)
    invalid_results = [(url, error) for url, is_valid, error in results if not is_valid]
    if invalid_results:
        print(f"❌ Found {len(invalid_results)} broken URLs:")
        for url, error in invalid_results:
            print(f"  - {url}: {error}")
        print()
        print(f"Summary: {valid_count}/{len(urls)} URLs valid")
        sys.exit(1)
    else:
        print(f"✅ All {len(urls)} URLs are valid!")
        sys.exit(0)

if __name__ == "__main__":
    main()
