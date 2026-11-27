"""Characterization tests for pseudo_label helpers and CUDA path setup."""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

import pytest

from recap_subworker.learning_machine.scripts import pseudo_label

MIN_CHARS_TEST = 50
EXPECTED_ACCEPT_TOP_PERCENT = 10.0
SHORT_TEXT = "短いテキスト"
JA_DOMINANT_TEXT = "あ" * 60 + "abc"
EN_DOMINANT_TEXT = "hello world " * 6
JA_BELOW_THRESHOLD_MIXED = "あ" * 2 + "a" * 10 + "1" * 50
EN_BELOW_THRESHOLD_MIXED = "a" * 10 + "1" * 50
NUMERIC_ONLY_TEXT = "1234567890" * 6
PUNCTUATION_ONLY_TEXT = "!!!" * 20


@pytest.mark.parametrize(
    ("text", "expected"),
    [
        (SHORT_TEXT, "unknown"),
        (PUNCTUATION_ONLY_TEXT, "unknown"),
        (JA_DOMINANT_TEXT, "ja"),
        (EN_DOMINANT_TEXT, "en"),
        (JA_BELOW_THRESHOLD_MIXED, "ja"),
        (EN_BELOW_THRESHOLD_MIXED, "en"),
        (NUMERIC_ONLY_TEXT, "unknown"),
    ],
)
def test_detect_language_simple_characterization(text: str, expected: str) -> None:
    assert pseudo_label.detect_language_simple(text, min_chars=MIN_CHARS_TEST) == expected


def test_setup_cuda_library_path_probes_expected_order(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    usr_local = tmp_path / "usr_local"
    cuda_ver_lib = usr_local / "cuda-12.4" / "targets" / "x86_64-linux" / "lib"
    cuda_ver_lib.mkdir(parents=True)
    cuda_link = tmp_path / "cuda_link"
    cuda_link_lib = cuda_link / "targets" / "x86_64-linux" / "lib"
    cuda_link_lib.mkdir(parents=True)
    sys_lib_a = tmp_path / "sys_lib_a"
    sys_lib_a.mkdir()
    sys_lib_b = tmp_path / "sys_lib_b"
    sys_lib_b.mkdir()
    ldconfig_dir = tmp_path / "ldconfig_cuda"
    ldconfig_dir.mkdir()
    ldconfig_so = ldconfig_dir / "libcudart.so"
    ldconfig_so.write_text("", encoding="utf-8")

    path_map = {
        "/usr/local": usr_local,
        "/usr/local/cuda": cuda_link,
        "/usr/lib/x86_64-linux-gnu": sys_lib_a,
        "/usr/lib64": sys_lib_b,
    }
    real_path = Path

    def _mapped_path(arg: str | Path) -> Path:
        key = str(arg)
        return path_map.get(key, real_path(arg))

    monkeypatch.setattr(pseudo_label, "Path", _mapped_path)
    monkeypatch.setattr(pseudo_label.shutil, "which", lambda _cmd: "/sbin/ldconfig")
    monkeypatch.setattr(
        pseudo_label.subprocess,
        "run",
        lambda *_args, **_kwargs: subprocess.CompletedProcess(
            args=["/sbin/ldconfig", "-p"],
            returncode=0,
            stdout=f"libcudart.so.12 (libc6,x86-64) => {ldconfig_so}\n",
            stderr="",
        ),
    )
    monkeypatch.setenv("LD_LIBRARY_PATH", f"{sys_lib_a}:/existing/path")

    pseudo_label._setup_cuda_library_path()

    expected_prefix = [
        str(cuda_ver_lib),
        str(cuda_link_lib),
        str(sys_lib_b),
        str(ldconfig_dir),
    ]
    assert pseudo_label.os.environ["LD_LIBRARY_PATH"] == ":".join(
        [*expected_prefix, f"{sys_lib_a}:/existing/path"]
    )


def test_parse_args_accept_top_percent_defaults(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(sys, "argv", ["pseudo_label.py", "--accept_top_percent", "10"])

    args = pseudo_label._parse_args()

    assert args.accept_top_percent == EXPECTED_ACCEPT_TOP_PERCENT
    assert args.model_dir == "recap_subworker/learning_machine/artifacts/teacher/v0_ja"
    assert args.output_path == "recap_subworker/learning_machine/data/silver_teacher_v0_ja.jsonl"


def test_parse_args_help_displays_escaped_percent(
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    monkeypatch.setattr(sys, "argv", ["pseudo_label.py", "--help"])

    with pytest.raises(SystemExit) as exc_info:
        pseudo_label._parse_args()

    assert exc_info.value.code == 0
    captured = capsys.readouterr()
    assert "Accept top P% of samples" in captured.out
