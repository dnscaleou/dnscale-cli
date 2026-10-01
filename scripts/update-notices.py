#!/usr/bin/env python3
"""Collect license texts for dependencies linked on the five release targets."""
import json
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]

def main():
    env = dict(os.environ, GOWORK="off", GOTOOLCHAIN="go" + (ROOT / ".go-version").read_text().strip())
    subprocess.run(["go", "mod", "download"], cwd=ROOT, env=env, check=True)
    decoder = json.JSONDecoder()
    modules = {}
    for target_os, arch in (("linux", "amd64"), ("linux", "arm64"), ("darwin", "amd64"), ("darwin", "arm64"), ("windows", "amd64")):
        target_env = dict(env, GOOS=target_os, GOARCH=arch, CGO_ENABLED="0")
        raw = subprocess.check_output(["go", "list", "-mod=readonly", "-deps", "-json", "./cmd/dnscale"], cwd=ROOT, env=target_env, text=True)
        while raw.strip():
            package, end = decoder.raw_decode(raw.lstrip())
            raw = raw.lstrip()[end:]
            module = package.get("Module")
            if module and not module.get("Main"):
                modules[module["Path"]] = module
    notices = ["DNScale CLI third-party notices\n\nFoundation and release tooling adapted from Postscale under MIT; see LICENSE.\n"]
    for module in sorted(modules.values(), key=lambda item: item["Path"]):
        directory = Path(module["Dir"])
        licenses = sorted(path for path in directory.iterdir() if path.is_file() and path.name.upper() in {"LICENSE", "LICENSE.TXT", "LICENSE.MD", "COPYING", "NOTICE", "NOTICE.TXT", "PATENTS"})
        if not licenses:
            raise SystemExit(f"missing license text for {module['Path']}")
        for path in licenses:
            notices.append(f"\n{module['Path']} {module['Version']} — {path.name}\n{'=' * 72}\n{path.read_text()}\n")
    goroot = Path(subprocess.check_output(["go", "env", "GOROOT"], cwd=ROOT, env=env, text=True).strip())
    version = (ROOT / ".go-version").read_text().strip()
    for name in ("LICENSE", "PATENTS"):
        path = goroot / name
        if path.exists():
            notices.append(f"\nGo {version} runtime — {name}\n{'=' * 72}\n{path.read_text()}\n")
    (ROOT / "THIRD_PARTY_NOTICES.txt").write_text("".join(notices).rstrip() + "\n")

if __name__ == "__main__":
    main()
