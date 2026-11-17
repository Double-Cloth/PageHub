#!/usr/bin/env python3
"""记录发布包实际来源，开发工作区同时记录 dirty 状态。"""
import json
from pathlib import Path
import subprocess
import shutil
import sys

source, destination = map(Path, sys.argv[1:])
saved = source / "source-version.json"
if saved.is_file():
    version = json.loads(saved.read_text())
elif not shutil.which("git"):
    version = {"commit": "", "dirty": True}
else:
    result = subprocess.run(["git", "-C", str(source), "rev-parse", "HEAD"], capture_output=True, text=True)
    commit = result.stdout.strip() if result.returncode == 0 else ""
    status = subprocess.run(["git", "-C", str(source), "status", "--porcelain"], capture_output=True, text=True)
    version = {"commit": commit, "dirty": status.returncode != 0 or bool(status.stdout)}
destination.write_text(json.dumps(version) + "\n")
