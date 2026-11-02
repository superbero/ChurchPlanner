#!/usr/bin/env python3
"""
Simple Git Auto-Push Script
Usage:
  python git_push.py               # Uses timestamp as commit message
  python git_push.py "Fixed bug"   # Uses custom commit message
"""

import subprocess
import sys
from datetime import datetime

def run_cmd(cmd):
    """Runs a shell command and returns True if successful."""
    try:
        print(f"  -> {' '.join(cmd)}")
        result = subprocess.run(cmd, check=True, text=True, capture_output=True)
        return True
    except subprocess.CalledProcessError as e:
        # Sometimes git outputs warnings to stderr even on success (e.g. CRLF issues)
        if e.stdout:
            print(e.stdout.strip())
        if e.stderr:
            print(e.stderr.strip())
        return False

def main():
    # 1. Determine the commit message
    if len(sys.argv) > 1:
        commit_msg = " ".join(sys.argv[1:])
    else:
        commit_msg = f"Auto-commit: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}"

    print(f"🚀 Starting Git Push ({commit_msg})")

    # 2. Git Add
    print("\n[1/3] Staging files...")
    if not run_cmd(["git", "add", "."]):
        print("❌ Failed to stage files.")
        sys.exit(1)

    # 3. Git Commit
    print("\n[2/3] Committing...")
    try:
        result = subprocess.run(
            ["git", "commit", "-m", commit_msg],
            text=True, capture_output=True
        )
        if result.returncode != 0:
            if "nothing to commit" in result.stdout:
                print("🤷 No changes to commit. Proceeding to push anyway...")
            else:
                print(result.stdout)
                print(result.stderr)
                print("❌ Commit failed.")
                sys.exit(1)
        else:
            print(result.stdout.strip())
    except Exception as e:
        print(f"❌ Error running git commit: {e}")
        sys.exit(1)

    # 4. Git Push
    print("\n[3/3] Pushing to remote...")
    if run_cmd(["git", "push"]):
        print("\n✅ Successfully pushed to remote!")
    else:
        print("\n❌ Push failed. Check your connection or remote settings (e.g. 'git push -u origin main').")
        sys.exit(1)

if __name__ == "__main__":
    main()
