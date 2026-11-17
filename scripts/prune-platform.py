#!/usr/bin/env python3
"""清理通过健康验收的历史平台版本，保留当前版本和回滚引用。"""
from pathlib import Path
import shutil
import sys

root = Path(sys.argv[1]).resolve(strict=True)
count = int(sys.argv[2])
if not 2 <= count <= 100:
    raise SystemExit('保留版本数须为 2..100')
releases = root / 'releases'
if releases.resolve(strict=True) != releases:
    raise SystemExit('发行目录不能包含链接')
current = (root / 'current').resolve(strict=True)
protected = {current}
previous = root / 'state/previous-release'
if previous.is_file():
    protected.add(Path(previous.read_text().strip()).resolve(strict=True))
versions = [item for item in releases.iterdir()
            if item.is_dir() and not item.is_symlink() and (item / '.pagehub-verified').is_file()]
versions.sort(key=lambda item: (item / '.pagehub-verified').stat().st_mtime_ns, reverse=True)
for target in versions[count:]:
    resolved = target.resolve(strict=True)
    if resolved.parent != releases or resolved != target or resolved in protected:
        continue
    shutil.rmtree(resolved)
