import os
import sys
import pytest
from unittest.mock import patch, MagicMock
import urllib.error
import hashlib

# Add the parent directory to sys.path so we can import the script
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), '..', 'ci')))
import importlib
pact_artifacts = importlib.import_module("pact-artifacts")

def test_download_and_verify_success(tmp_path):
    data = b"fake-data"
    expected_hash = hashlib.sha256(data).hexdigest()

    mock_response = MagicMock()
    mock_response.read.side_effect = [data, b""]
    mock_response.__enter__.return_value = mock_response

    dest_path = str(tmp_path / "test.file")

    with patch("urllib.request.urlopen", return_value=mock_response):
        pact_artifacts.download_and_verify("https://github.com/pact-foundation/file", expected_hash, dest_path)

    assert os.path.exists(dest_path)
    with open(dest_path, "rb") as f:
        assert f.read() == b"fake-data"

def test_download_and_verify_hash_mismatch(tmp_path):
    data = b"fake-data"
    # Actual valid 64-hex incorrect digest (not matching data's sha256)
    expected_hash = "0" * 64

    mock_response = MagicMock()
    mock_response.read.side_effect = [data, b""]
    mock_response.__enter__.return_value = mock_response

    dest_path = str(tmp_path / "test.file")

    with patch("urllib.request.urlopen", return_value=mock_response):
        with pytest.raises(SystemExit) as exc:
            pact_artifacts.download_and_verify("https://github.com/pact-foundation/file", expected_hash, dest_path)
        assert exc.value.code == 1

    # Verify content mismatch check occurs BEFORE file publish (neither final nor temp file exists)
    assert not os.path.exists(dest_path)
    assert not os.path.exists(dest_path + ".tmp")

def test_download_and_verify_http_error(tmp_path):
    dest_path = str(tmp_path / "test.file")

    with patch("urllib.request.urlopen", side_effect=urllib.error.URLError("Not found")):
        with pytest.raises(SystemExit) as exc:
            pact_artifacts.download_and_verify("https://github.com/pact-foundation/file", "a"*64, dest_path)
        assert exc.value.code == 1

    assert not os.path.exists(dest_path)


def test_download_and_verify_invalid_url(tmp_path):
    dest_path = str(tmp_path / "test.file")
    with pytest.raises(SystemExit) as exc:
        pact_artifacts.download_and_verify("http://evil.com/file", "a"*64, dest_path)
    assert exc.value.code == 1

def test_download_and_verify_invalid_hash_format(tmp_path, capsys):
    dest_path = str(tmp_path / "test.file")
    # Short
    with pytest.raises(SystemExit) as exc:
        pact_artifacts.download_and_verify("https://github.com/pact-foundation/file", "short", dest_path)
    assert exc.value.code == 1
    assert "Invalid expected hash format!" in capsys.readouterr().out

    # Uppercase hex must be rejected by strict [0-9a-f]{64}
    with pytest.raises(SystemExit) as exc:
        pact_artifacts.download_and_verify("https://github.com/pact-foundation/file", "A" * 64, dest_path)
    assert exc.value.code == 1
    assert "Invalid expected hash format!" in capsys.readouterr().out

    # Non-hex characters
    with pytest.raises(SystemExit) as exc:
        pact_artifacts.download_and_verify("https://github.com/pact-foundation/file", "g" * 64, dest_path)
    assert exc.value.code == 1
    assert "Invalid expected hash format!" in capsys.readouterr().out

def test_main_hash_mismatch_prevents_extract(tmp_path, monkeypatch):
    data = b"corrupted-tarball"
    mock_response = MagicMock()
    mock_response.read.side_effect = [data, b""]
    mock_response.__enter__.return_value = mock_response

    extract_called = []
    monkeypatch.setattr(pact_artifacts, "extract_standalone", lambda *args, **kwargs: extract_called.append(True))
    monkeypatch.setattr(sys, "argv", ["pact-artifacts.py", "--type", "standalone"])
    monkeypatch.chdir(tmp_path)

    with patch("urllib.request.urlopen", return_value=mock_response):
        with pytest.raises(SystemExit) as exc:
            pact_artifacts.main()
        assert exc.value.code == 1

    assert not extract_called, "extract_standalone must NOT be called when hash mismatches"

def test_extract_standalone_escaping_symlink(tmp_path):
    import io
    import tarfile

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
    assert not (outside_dir / "probe.txt").exists(), "Symlink traversal must not write outside destination"

def test_extract_standalone_failclosed_when_data_filter_unavailable(tmp_path, monkeypatch):
    import io
    import tarfile

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

def test_download_and_verify_max_size(tmp_path):
    dest_path = str(tmp_path / "test.file")
    mock_response = MagicMock()
    mock_response.read.side_effect = [b"a" * (10 * 1024 * 1024)] * 6 + [b""]
    mock_response.__enter__.return_value = mock_response

    with patch("urllib.request.urlopen", return_value=mock_response):
        with pytest.raises(SystemExit) as exc:
            pact_artifacts.download_and_verify("https://github.com/pact-foundation/file", "a"*64, dest_path)
        assert exc.value.code == 1
