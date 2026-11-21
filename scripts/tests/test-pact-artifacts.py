from __future__ import annotations

import gzip
import hashlib
import http.client
import importlib
import io
import sys
import tarfile
import urllib.error
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

# Add the parent directory to sys.path so we can import the script
sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "ci"))

pact_artifacts = importlib.import_module("pact-artifacts")

MAX_ATTEMPTS = 3
TWO_ATTEMPTS = 2


def test_download_and_verify_success(tmp_path: Path) -> None:
    data = b"fake-data"
    expected_hash = hashlib.sha256(data).hexdigest()

    mock_response = MagicMock()
    mock_response.read.side_effect = [data, b""]
    mock_response.__enter__.return_value = mock_response

    dest_file = tmp_path / "test.file"
    dest_path = str(dest_file)

    with patch("urllib.request.urlopen", return_value=mock_response):
        pact_artifacts.download_and_verify(
            "https://github.com/pact-foundation/file", expected_hash, dest_path
        )

    assert dest_file.exists()
    with dest_file.open("rb") as f:
        assert f.read() == b"fake-data"


def test_download_and_verify_hash_mismatch(tmp_path: Path) -> None:
    data = b"fake-data"
    # Actual valid 64-hex incorrect digest (not matching data's sha256)
    expected_hash = "0" * 64

    mock_response = MagicMock()
    mock_response.read.side_effect = [data, b""]
    mock_response.__enter__.return_value = mock_response

    dest_file = tmp_path / "test.file"
    dest_path = str(dest_file)

    with patch("urllib.request.urlopen", return_value=mock_response):
        with pytest.raises(SystemExit) as exc:
            pact_artifacts.download_and_verify(
                "https://github.com/pact-foundation/file", expected_hash, dest_path
            )
        assert exc.value.code == 1

    # Verify content mismatch check occurs BEFORE file publish (neither final nor temp file exists)
    assert not dest_file.exists()
    assert not (tmp_path / "test.file.tmp").exists()


def test_download_and_verify_http_error(tmp_path: Path) -> None:
    dest_file = tmp_path / "test.file"
    dest_path = str(dest_file)

    with (
        patch("urllib.request.urlopen", side_effect=urllib.error.URLError("Not found")),
        patch("time.sleep"),
    ):
        with pytest.raises(SystemExit) as exc:
            pact_artifacts.download_and_verify(
                "https://github.com/pact-foundation/file", "a" * 64, dest_path
            )
        assert exc.value.code == 1

    assert not dest_file.exists()


def test_download_and_verify_invalid_url(tmp_path: Path) -> None:
    dest_path = str(tmp_path / "test.file")
    with pytest.raises(SystemExit) as exc:
        pact_artifacts.download_and_verify("http://evil.com/file", "a" * 64, dest_path)
    assert exc.value.code == 1


def test_download_and_verify_invalid_hash_format(
    tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    dest_path = str(tmp_path / "test.file")
    # Short
    with pytest.raises(SystemExit) as exc:
        pact_artifacts.download_and_verify(
            "https://github.com/pact-foundation/file", "short", dest_path
        )
    assert exc.value.code == 1
    assert "Invalid expected hash format!" in capsys.readouterr().out

    # Uppercase hex must be rejected by strict [0-9a-f]{64}
    with pytest.raises(SystemExit) as exc:
        pact_artifacts.download_and_verify(
            "https://github.com/pact-foundation/file", "A" * 64, dest_path
        )
    assert exc.value.code == 1
    assert "Invalid expected hash format!" in capsys.readouterr().out

    # Non-hex characters
    with pytest.raises(SystemExit) as exc:
        pact_artifacts.download_and_verify(
            "https://github.com/pact-foundation/file", "g" * 64, dest_path
        )
    assert exc.value.code == 1
    assert "Invalid expected hash format!" in capsys.readouterr().out


def test_main_hash_mismatch_prevents_extract(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    data = b"corrupted-tarball"
    mock_response = MagicMock()
    mock_response.read.side_effect = [data, b""]
    mock_response.__enter__.return_value = mock_response

    extract_called = []
    monkeypatch.setattr(
        pact_artifacts, "extract_standalone", lambda *args, **kwargs: extract_called.append(True)
    )
    monkeypatch.setattr(sys, "argv", ["pact-artifacts.py", "--type", "standalone"])
    monkeypatch.chdir(tmp_path)

    with patch("urllib.request.urlopen", return_value=mock_response):
        with pytest.raises(SystemExit) as exc:
            pact_artifacts.main()
        assert exc.value.code == 1

    assert not extract_called, "extract_standalone must NOT be called when hash mismatches"


def test_extract_standalone_escaping_symlink(tmp_path: Path) -> None:
    dest_dir = tmp_path / "dest"
    outside_dir = tmp_path / "outside"
    dest_dir.mkdir()
    outside_dir.mkdir()

    # Create tar with alias -> ../outside and alias/probe.txt
    tar_path = tmp_path / "malicious.tar.gz"
    with tarfile.open(tar_path, "w:gz") as tar:
        ti_link = tarfile.TarInfo(name="alias")
        ti_link.type = tarfile.SYMTYPE
        ti_link.linkname = "../outside"
        tar.addfile(ti_link)

        ti_file = tarfile.TarInfo(name="alias/probe.txt")
        content = b"escaped payload"
        ti_file.size = len(content)
        tar.addfile(ti_file, io.BytesIO(content))

    with pytest.raises(SystemExit) as exc:
        pact_artifacts.extract_standalone(str(tar_path), dest_dir=str(dest_dir))
    assert exc.value.code == 1
    assert not (outside_dir / "probe.txt").exists(), (
        "Symlink traversal must not write outside destination"
    )


def test_extract_standalone_failclosed_when_data_filter_unavailable(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    dest_dir = tmp_path / "dest"
    dest_dir.mkdir()

    tar_path = tmp_path / "sample.tar.gz"
    with tarfile.open(tar_path, "w:gz") as tar:
        ti_file = tarfile.TarInfo(name="test.txt")
        content = b"hello"
        ti_file.size = len(content)
        tar.addfile(ti_file, io.BytesIO(content))

    monkeypatch.delattr(tarfile, "data_filter", raising=False)

    with pytest.raises(SystemExit) as exc:
        pact_artifacts.extract_standalone(str(tar_path), dest_dir=str(dest_dir))
    assert exc.value.code == 1


def test_download_and_verify_max_size(tmp_path: Path) -> None:
    dest_path = str(tmp_path / "test.file")
    mock_response = MagicMock()
    mock_response.read.side_effect = [b"a" * (10 * 1024 * 1024)] * 6 + [b""]
    mock_response.__enter__.return_value = mock_response

    with patch("urllib.request.urlopen", return_value=mock_response):
        with pytest.raises(SystemExit) as exc:
            pact_artifacts.download_and_verify(
                "https://github.com/pact-foundation/file", "a" * 64, dest_path
            )
        assert exc.value.code == 1


def _make_mock_response(data: bytes) -> MagicMock:
    mock_response = MagicMock()
    mock_response.read.side_effect = [data, b""]
    mock_response.__enter__.return_value = mock_response
    return mock_response


def test_download_and_verify_retries_transient_url_error_with_backoff(tmp_path: Path) -> None:
    url = "https://github.com/pact-foundation/file"
    data = b"recovered-after-urlerror"
    expected_hash = hashlib.sha256(data).hexdigest()
    dest_file = tmp_path / "test.file"
    dest_path = str(dest_file)

    side_effects = [
        urllib.error.URLError("connection reset"),
        urllib.error.URLError("temporary DNS failure"),
        _make_mock_response(data),
    ]

    with (
        patch("urllib.request.urlopen", side_effect=side_effects) as mock_urlopen,
        patch("time.sleep") as mock_sleep,
    ):
        pact_artifacts.download_and_verify(url, expected_hash, dest_path)

    assert mock_urlopen.call_count == MAX_ATTEMPTS
    assert [call.args[0] for call in mock_sleep.call_args_list] == [5.0, 10.0]
    with dest_file.open("rb") as f:
        assert f.read() == data


def test_download_and_verify_retries_http_5xx_with_backoff(tmp_path: Path) -> None:
    url = "https://github.com/pact-foundation/file"
    data = b"recovered-after-5xx"
    expected_hash = hashlib.sha256(data).hexdigest()
    dest_file = tmp_path / "test.file"
    dest_path = str(dest_file)

    side_effects = [
        urllib.error.HTTPError(url, 502, "Bad Gateway", hdrs=None, fp=None),
        urllib.error.HTTPError(url, 503, "Service Unavailable", hdrs=None, fp=None),
        _make_mock_response(data),
    ]

    with (
        patch("urllib.request.urlopen", side_effect=side_effects) as mock_urlopen,
        patch("time.sleep") as mock_sleep,
    ):
        pact_artifacts.download_and_verify(url, expected_hash, dest_path)

    assert mock_urlopen.call_count == MAX_ATTEMPTS
    assert [call.args[0] for call in mock_sleep.call_args_list] == [5.0, 10.0]
    with dest_file.open("rb") as f:
        assert f.read() == data


def test_download_and_verify_retries_timeout_with_backoff(tmp_path: Path) -> None:
    url = "https://github.com/pact-foundation/file"
    data = b"recovered-after-timeout"
    expected_hash = hashlib.sha256(data).hexdigest()
    dest_file = tmp_path / "test.file"
    dest_path = str(dest_file)

    side_effects = [
        TimeoutError("read timed out"),
        _make_mock_response(data),
    ]

    with (
        patch("urllib.request.urlopen", side_effect=side_effects) as mock_urlopen,
        patch("time.sleep") as mock_sleep,
    ):
        pact_artifacts.download_and_verify(url, expected_hash, dest_path)

    assert mock_urlopen.call_count == TWO_ATTEMPTS
    assert [call.args[0] for call in mock_sleep.call_args_list] == [5.0]
    with dest_file.open("rb") as f:
        assert f.read() == data


def test_download_and_verify_exhausts_retries_and_fails(tmp_path: Path) -> None:
    url = "https://github.com/pact-foundation/file"
    dest_file = tmp_path / "test.file"
    dest_path = str(dest_file)

    err = urllib.error.HTTPError(url, 503, "Service Unavailable", hdrs=None, fp=None)

    with (
        patch("urllib.request.urlopen", side_effect=err) as mock_urlopen,
        patch("time.sleep") as mock_sleep,
    ):
        with pytest.raises(SystemExit) as exc:
            pact_artifacts.download_and_verify(url, "a" * 64, dest_path)
        assert exc.value.code == 1

    assert mock_urlopen.call_count == MAX_ATTEMPTS
    assert [call.args[0] for call in mock_sleep.call_args_list] == [5.0, 10.0]
    assert not dest_file.exists()


def test_download_and_verify_does_not_retry_http_4xx(tmp_path: Path) -> None:
    url = "https://github.com/pact-foundation/file"
    data = b"should-not-be-reached"
    expected_hash = hashlib.sha256(data).hexdigest()
    dest_file = tmp_path / "test.file"
    dest_path = str(dest_file)

    side_effects = [
        urllib.error.HTTPError(url, 503, "Service Unavailable", hdrs=None, fp=None),
        urllib.error.HTTPError(url, 404, "Not Found", hdrs=None, fp=None),
        _make_mock_response(data),
    ]

    with (
        patch("urllib.request.urlopen", side_effect=side_effects) as mock_urlopen,
        patch("time.sleep") as mock_sleep,
    ):
        with pytest.raises(SystemExit) as exc:
            pact_artifacts.download_and_verify(url, expected_hash, dest_path)
        assert exc.value.code == 1

    assert mock_urlopen.call_count == TWO_ATTEMPTS
    assert [call.args[0] for call in mock_sleep.call_args_list] == [5.0]
    assert not dest_file.exists()


def test_download_and_verify_retries_mid_read_connection_reset(tmp_path: Path) -> None:
    url = "https://github.com/pact-foundation/file"
    data = b"recovered-after-connection-reset"
    expected_hash = hashlib.sha256(data).hexdigest()
    dest_file = tmp_path / "test.file"
    dest_path = str(dest_file)

    broken_response = MagicMock()
    broken_response.read.side_effect = [
        b"partial-",
        ConnectionResetError("Connection reset by peer"),
    ]
    broken_response.__enter__.return_value = broken_response

    side_effects = [
        broken_response,
        _make_mock_response(data),
    ]

    with (
        patch("urllib.request.urlopen", side_effect=side_effects) as mock_urlopen,
        patch("time.sleep") as mock_sleep,
    ):
        pact_artifacts.download_and_verify(url, expected_hash, dest_path)

    assert mock_urlopen.call_count == TWO_ATTEMPTS
    assert [call.args[0] for call in mock_sleep.call_args_list] == [5.0]
    with dest_file.open("rb") as f:
        assert f.read() == data


def test_download_and_verify_retries_mid_read_incomplete_read(tmp_path: Path) -> None:
    url = "https://github.com/pact-foundation/file"
    data = b"recovered-after-incomplete-read"
    expected_hash = hashlib.sha256(data).hexdigest()
    dest_file = tmp_path / "test.file"
    dest_path = str(dest_file)

    broken_response_1 = MagicMock()
    broken_response_1.read.side_effect = [b"part1", http.client.IncompleteRead(b"part2", 128)]
    broken_response_1.__enter__.return_value = broken_response_1

    broken_response_2 = MagicMock()
    broken_response_2.read.side_effect = [http.client.IncompleteRead(b"part1", 64)]
    broken_response_2.__enter__.return_value = broken_response_2

    side_effects = [
        broken_response_1,
        broken_response_2,
        _make_mock_response(data),
    ]

    with (
        patch("urllib.request.urlopen", side_effect=side_effects) as mock_urlopen,
        patch("time.sleep") as mock_sleep,
    ):
        pact_artifacts.download_and_verify(url, expected_hash, dest_path)

    assert mock_urlopen.call_count == MAX_ATTEMPTS
    assert [call.args[0] for call in mock_sleep.call_args_list] == [5.0, 10.0]
    with dest_file.open("rb") as f:
        assert f.read() == data


def test_ffi_fresh_install_writes_library_stamp_and_no_pact_download(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    payload = b"fake-libpact-ffi-binary-v1"
    gz_bytes = gzip.compress(payload)
    pin = hashlib.sha256(gz_bytes).hexdigest()

    home_dir = tmp_path / "home"
    home_dir.mkdir()
    work_dir = tmp_path / "workdir"
    work_dir.mkdir()

    monkeypatch.setenv("HOME", str(home_dir))
    monkeypatch.chdir(work_dir)
    monkeypatch.setattr(pact_artifacts, "LIBPACT_FFI_SHA256", pin)
    monkeypatch.setattr(sys, "argv", ["pact-artifacts.py", "--type", "ffi"])

    with patch("urllib.request.urlopen", return_value=_make_mock_response(gz_bytes)):
        pact_artifacts.main()

    lib_dir = home_dir / ".pact" / "lib"
    assert (lib_dir / "libpact_ffi.so").read_bytes() == payload
    assert (lib_dir / "libpact_ffi.so.sha256").read_text(encoding="utf-8").strip() == pin
    assert not (work_dir / "pact_download").exists()


def test_ffi_skips_download_when_stamp_matches(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    payload = b"existing-pinned-libpact-ffi"
    pin = "a" * 64

    home_dir = tmp_path / "home"
    lib_dir = home_dir / ".pact" / "lib"
    lib_dir.mkdir(parents=True)
    ffi_so = lib_dir / "libpact_ffi.so"
    stamp_file = lib_dir / "libpact_ffi.so.sha256"
    ffi_so.write_bytes(payload)
    stamp_file.write_text(f"{pin}\n", encoding="utf-8")

    monkeypatch.setenv("HOME", str(home_dir))
    monkeypatch.chdir(tmp_path)
    monkeypatch.setattr(pact_artifacts, "LIBPACT_FFI_SHA256", pin)
    monkeypatch.setattr(sys, "argv", ["pact-artifacts.py", "--type", "ffi"])

    with patch(
        "urllib.request.urlopen",
        side_effect=AssertionError("urlopen must not be called when stamp matches"),
    ) as mock_urlopen:
        pact_artifacts.main()

    mock_urlopen.assert_not_called()
    assert ffi_so.read_bytes() == payload
    out = capsys.readouterr().out
    assert pin[:12] in out


def test_ffi_redownloads_when_stamp_differs(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    new_payload = b"new-libpact-ffi-binary-v2"
    gz_bytes = gzip.compress(new_payload)
    new_pin = hashlib.sha256(gz_bytes).hexdigest()

    home_dir = tmp_path / "home"
    lib_dir = home_dir / ".pact" / "lib"
    lib_dir.mkdir(parents=True)
    ffi_so = lib_dir / "libpact_ffi.so"
    stamp_file = lib_dir / "libpact_ffi.so.sha256"
    ffi_so.write_bytes(b"old-libpact-ffi")
    stamp_file.write_text("0" * 64 + "\n", encoding="utf-8")

    monkeypatch.setenv("HOME", str(home_dir))
    monkeypatch.chdir(tmp_path)
    monkeypatch.setattr(pact_artifacts, "LIBPACT_FFI_SHA256", new_pin)
    monkeypatch.setattr(sys, "argv", ["pact-artifacts.py", "--type", "ffi"])

    with patch(
        "urllib.request.urlopen", return_value=_make_mock_response(gz_bytes)
    ) as mock_urlopen:
        pact_artifacts.main()

    assert mock_urlopen.call_count == 1
    assert ffi_so.read_bytes() == new_payload
    assert stamp_file.read_text(encoding="utf-8").strip() == new_pin


def test_ffi_redownloads_when_stamp_present_but_library_missing(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    payload = b"restored-libpact-ffi"
    gz_bytes = gzip.compress(payload)
    pin = hashlib.sha256(gz_bytes).hexdigest()

    home_dir = tmp_path / "home"
    lib_dir = home_dir / ".pact" / "lib"
    lib_dir.mkdir(parents=True)
    stamp_file = lib_dir / "libpact_ffi.so.sha256"
    stamp_file.write_text(f"{pin}\n", encoding="utf-8")

    monkeypatch.setenv("HOME", str(home_dir))
    monkeypatch.chdir(tmp_path)
    monkeypatch.setattr(pact_artifacts, "LIBPACT_FFI_SHA256", pin)
    monkeypatch.setattr(sys, "argv", ["pact-artifacts.py", "--type", "ffi"])

    with patch(
        "urllib.request.urlopen", return_value=_make_mock_response(gz_bytes)
    ) as mock_urlopen:
        pact_artifacts.main()

    assert mock_urlopen.call_count == 1
    assert (lib_dir / "libpact_ffi.so").read_bytes() == payload
    assert stamp_file.read_text(encoding="utf-8").strip() == pin


def test_ffi_checksum_mismatch_preserves_existing_library_and_stamp(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    old_payload = b"previous-good-lib"
    old_pin = "1" * 64
    wrong_pin = "2" * 64
    bad_gz = gzip.compress(b"unexpected-bytes")

    home_dir = tmp_path / "home"
    lib_dir = home_dir / ".pact" / "lib"
    lib_dir.mkdir(parents=True)
    ffi_so = lib_dir / "libpact_ffi.so"
    stamp_file = lib_dir / "libpact_ffi.so.sha256"
    ffi_so.write_bytes(old_payload)
    stamp_file.write_text(f"{old_pin}\n", encoding="utf-8")

    monkeypatch.setenv("HOME", str(home_dir))
    monkeypatch.chdir(tmp_path)
    monkeypatch.setattr(pact_artifacts, "LIBPACT_FFI_SHA256", wrong_pin)
    monkeypatch.setattr(sys, "argv", ["pact-artifacts.py", "--type", "ffi"])

    with patch("urllib.request.urlopen", return_value=_make_mock_response(bad_gz)):
        with pytest.raises(SystemExit) as exc:
            pact_artifacts.main()
        assert exc.value.code != 0

    assert ffi_so.read_bytes() == old_payload
    assert stamp_file.read_text(encoding="utf-8").strip() == old_pin


def test_ffi_corrupt_gzip_preserves_existing_library_and_writes_no_new_stamp(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    old_payload = b"previous-good-lib"
    old_pin = "1" * 64
    # Truncated gzip stream that still matches the patched archive checksum
    corrupt_gz = gzip.compress(b"some-payload-bytes")[:10]
    corrupt_pin = hashlib.sha256(corrupt_gz).hexdigest()

    home_dir = tmp_path / "home"
    lib_dir = home_dir / ".pact" / "lib"
    lib_dir.mkdir(parents=True)
    ffi_so = lib_dir / "libpact_ffi.so"
    stamp_file = lib_dir / "libpact_ffi.so.sha256"
    ffi_so.write_bytes(old_payload)
    stamp_file.write_text(f"{old_pin}\n", encoding="utf-8")

    monkeypatch.setenv("HOME", str(home_dir))
    monkeypatch.chdir(tmp_path)
    monkeypatch.setattr(pact_artifacts, "LIBPACT_FFI_SHA256", corrupt_pin)
    monkeypatch.setattr(sys, "argv", ["pact-artifacts.py", "--type", "ffi"])

    with patch("urllib.request.urlopen", return_value=_make_mock_response(corrupt_gz)):
        with pytest.raises(SystemExit) as exc:
            pact_artifacts.main()
        assert exc.value.code != 0

    assert ffi_so.read_bytes() == old_payload
    assert stamp_file.read_text(encoding="utf-8").strip() == old_pin
    assert sorted(p.name for p in lib_dir.iterdir()) == ["libpact_ffi.so", "libpact_ffi.so.sha256"]
