#!/usr/bin/env python3
"""Build script for TinyMD (the WebView2 editor in webview/).

Usage:
    python build.py              # dev build (console window kept for crash output)
    python build.py --prod       # production build (optimized, no console)
"""

import argparse
import os
import subprocess
import sys
import time

SRC_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "webview")
EXE = "tinymd-webview.exe"


def build(prod=False):
    exe_path = os.path.join(SRC_DIR, EXE)
    cmd = ["go", "build"]
    if prod:
        cmd += ["-ldflags", "-s -w -H windowsgui", "-trimpath"]
    cmd += ["-o", exe_path, "."]

    mode = "prod" if prod else "dev"
    print(f"  [{mode}] WebView2 -> {EXE}", end="", flush=True)
    start = time.time()
    result = subprocess.run(cmd, cwd=SRC_DIR, capture_output=True, text=True)
    elapsed = time.time() - start
    if result.returncode != 0:
        print(f"  FAILED ({elapsed:.1f}s)")
        print(result.stderr)
        return False
    size_mb = os.path.getsize(exe_path) / (1024 * 1024)
    print(f"  {size_mb:.1f} MB  ({elapsed:.1f}s)")
    return True


def main():
    parser = argparse.ArgumentParser(description="Build TinyMD")
    parser.add_argument("--prod", action="store_true",
                        help="Production build: strip symbols, hide console window")
    args = parser.parse_args()
    sys.exit(0 if build(args.prod) else 1)


if __name__ == "__main__":
    main()
