"""Run the Go harness suite when a checker script is only an exec wrapper."""

from __future__ import annotations

import os
import pathlib
import shutil
import subprocess
import sys


def delegated(script: pathlib.Path, packages: list[str], cli: list[str]) -> int | None:
    """Return an exit code when ``script`` execs alt-harness, else None."""
    try:
        text = script.read_text(encoding="utf-8")
    except OSError:
        return None
    if "os.execv" not in text or "alt-harness" not in text:
        return None
    root = script.resolve().parents[2]
    env = os.environ.copy()
    env["PATH"] = str(pathlib.Path.home() / ".local" / "bin") + os.pathsep + env.get("PATH", "")
    rc = 0
    mod = root / "scripts" / "altharness"
    go = shutil.which("go")
    if go is None:
        sys.stderr.write("go executable not found on PATH\n")
        return 1
    for pkg in packages:
        proc = subprocess.run(  # noqa: S603 -- fixed go test argv from shutil.which
            [go, "test", "-count=1", pkg],
            cwd=str(mod),
            env=env,
            check=False,
        )
        if proc.returncode != 0:
            rc = proc.returncode
    if cli:
        launcher = root / "scripts" / "altharness" / "alt-harness"
        proc = subprocess.run(  # noqa: S603 -- fixed launcher path inside the repo
            [str(launcher), *cli],
            cwd=str(root),
            env=env,
            check=False,
        )
        if proc.returncode != 0:
            rc = proc.returncode
    sys.stdout.write(f"\ngo harness delegate: packages={packages} cli={cli} rc={rc}\n")
    return rc
