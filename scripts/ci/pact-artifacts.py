import argparse
import gzip
import hashlib
import http.client
import os
import re
import shutil
import sys
import tarfile
import time
import urllib.error
import urllib.request
from pathlib import Path

PACT_STANDALONE_URL = "https://github.com/pact-foundation/pact-standalone/releases/download/v2.4.10/pact-2.4.10-linux-x86_64.tar.gz"
# SHA256 derived after verifying official SHA1 ba65cba9607f8c636ea6abff65539ac5766ad48d
PACT_STANDALONE_SHA256 = "6a48f4c00049dbb5f246c04caac166aaeab20745f5a4c416112520105fb84c2d"

LIBPACT_FFI_URL = "https://github.com/pact-foundation/pact-reference/releases/download/libpact_ffi-v0.4.28/libpact_ffi-linux-x86_64.so.gz"
# Official SHA256 from https://github.com/pact-foundation/pact-reference/releases/expanded_assets/libpact_ffi-v0.4.28
LIBPACT_FFI_SHA256 = "73053bd1b3219e7012ec1f7ee0cc7bbb94a07bb44249d8f4002be0ba4bddbd4e"

MAX_DOWNLOAD_ATTEMPTS = 3
INITIAL_BACKOFF_SECONDS = 5.0
HTTP_SERVER_ERROR_MIN = 500
HTTP_SERVER_ERROR_MAX = 600


def _fetch_bytes_with_retry(
    req: urllib.request.Request,
    max_attempts: int = MAX_DOWNLOAD_ATTEMPTS,
    initial_backoff: float = INITIAL_BACKOFF_SECONDS,
) -> bytes:
    if max_attempts < 1:
        msg = "max_attempts must be >= 1"
        raise ValueError(msg)
    for attempt in range(1, max_attempts + 1):
        try:
            # bounded network read
            with urllib.request.urlopen(req, timeout=30) as response:  # noqa: S310 — URL restricted to https://github.com/pact-foundation/ in download_and_verify
                data = b""
                while True:
                    chunk = response.read(8192)
                    if not chunk:
                        break
                    data += chunk
                    if len(data) > 50 * 1024 * 1024:
                        sys.stdout.write("File too large! Max 50MB allowed.\n")
                        sys.exit(1)
                return data
        except urllib.error.HTTPError as e:
            if HTTP_SERVER_ERROR_MIN <= e.code < HTTP_SERVER_ERROR_MAX and attempt < max_attempts:
                delay = initial_backoff * (2 ** (attempt - 1))
                sys.stdout.write(
                    f"Transient HTTP {e.code} on attempt {attempt}/{max_attempts}, retrying in {delay}s...\n"
                )
                time.sleep(delay)
                continue
            raise
        except (
            urllib.error.URLError,
            TimeoutError,
            ConnectionError,
            http.client.IncompleteRead,
        ) as e:
            if attempt < max_attempts:
                delay = initial_backoff * (2 ** (attempt - 1))
                sys.stdout.write(
                    f"Transient download error ({e}) on attempt {attempt}/{max_attempts}, retrying in {delay}s...\n"
                )
                time.sleep(delay)
                continue
            raise
    raise RuntimeError("unreachable")


def download_and_verify(url: str, expected_hash: str, dest_path: str | os.PathLike[str]) -> None:
    if not url.startswith("https://github.com/pact-foundation/"):
        sys.stdout.write("Invalid URL domain!\n")
        sys.exit(1)
    if not re.fullmatch(r"[0-9a-f]{64}", expected_hash):
        sys.stdout.write("Invalid expected hash format!\n")
        sys.exit(1)

    sys.stdout.write(f"Downloading {url}...\n")
    req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})  # noqa: S310 — URL scheme/prefix validated to https://github.com/pact-foundation/ above
    try:
        data = _fetch_bytes_with_retry(req)
    except (urllib.error.URLError, TimeoutError, ConnectionError, http.client.IncompleteRead) as e:
        sys.stdout.write(f"Failed to download: {e}\n")
        sys.exit(1)

    h = hashlib.sha256()
    h.update(data)
    actual_hash = h.hexdigest()

    if actual_hash != expected_hash:
        sys.stdout.write(f"Hash mismatch for {url}!\n")
        sys.stdout.write(f"Expected (SHA256): {expected_hash}\n")
        sys.stdout.write(f"Actual   (SHA256): {actual_hash}\n")
        sys.exit(1)

    sys.stdout.write(f"Hash verified (SHA256): {actual_hash}, Size: {len(data)} bytes\n")

    # Save to a temporary file instead of deleting on error
    dest = Path(dest_path)
    tmp = Path(f"{dest_path}.tmp")
    with tmp.open("wb") as f:
        f.write(data)
    tmp.rename(dest)


def extract_standalone(
    tgz_path: str | os.PathLike[str], dest_dir: str | os.PathLike[str] = "."
) -> None:
    sys.stdout.write("Extracting pact-standalone...\n")
    if not hasattr(tarfile, "data_filter"):
        sys.stdout.write("tarfile.data_filter unavailable; refusing extraction (fail-closed)\n")
        sys.exit(1)
    with tarfile.open(tgz_path, "r:gz") as tar:
        try:
            tar.extractall(path=dest_dir, filter="data")
        except (tarfile.TarError, OSError) as e:
            sys.stdout.write(f"Safe extraction failed: {e}\n")
            sys.exit(1)


def extract_ffi(gz_path: str | os.PathLike[str]) -> None:
    home_dir = Path(os.environ.get("HOME", "."))
    lib_dir = home_dir / ".pact" / "lib"
    lib_dir.mkdir(parents=True, exist_ok=True)
    ffi_so = lib_dir / "libpact_ffi.so"
    sys.stdout.write(f"Extracting libpact_ffi to {ffi_so}...\n")
    with gzip.open(gz_path, "rb") as f_in, ffi_so.open("wb") as f_out:
        shutil.copyfileobj(f_in, f_out)


def main() -> None:
    parser = argparse.ArgumentParser(description="Download and verify Pact artifacts securely.")
    parser.add_argument(
        "--type", choices=["standalone", "ffi"], required=True, help="Artifact type to download"
    )
    args = parser.parse_args()

    Path("pact_download").mkdir(parents=True, exist_ok=True)

    if args.type == "standalone":
        standalone_tgz = "pact_download/pact-standalone.tar.gz"
        download_and_verify(PACT_STANDALONE_URL, PACT_STANDALONE_SHA256, standalone_tgz)
        extract_standalone(standalone_tgz)
    elif args.type == "ffi":
        ffi_gz = "pact_download/libpact_ffi.so.gz"
        download_and_verify(LIBPACT_FFI_URL, LIBPACT_FFI_SHA256, ffi_gz)
        extract_ffi(ffi_gz)

    sys.stdout.write(f"Artifact '{args.type}' downloaded and verified successfully.\n")


if __name__ == "__main__":
    main()
