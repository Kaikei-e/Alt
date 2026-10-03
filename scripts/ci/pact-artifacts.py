import hashlib
import os
import re
import sys
import tarfile
import urllib.request
import urllib.error
import gzip
import shutil
import argparse

PACT_STANDALONE_URL = "https://github.com/pact-foundation/pact-standalone/releases/download/v2.4.10/pact-2.4.10-linux-x86_64.tar.gz"
# SHA256 derived after verifying official SHA1 ba65cba9607f8c636ea6abff65539ac5766ad48d
PACT_STANDALONE_SHA256 = "6a48f4c00049dbb5f246c04caac166aaeab20745f5a4c416112520105fb84c2d"

LIBPACT_FFI_URL = "https://github.com/pact-foundation/pact-reference/releases/download/libpact_ffi-v0.4.28/libpact_ffi-linux-x86_64.so.gz"
# Official SHA256 from https://github.com/pact-foundation/pact-reference/releases/expanded_assets/libpact_ffi-v0.4.28
LIBPACT_FFI_SHA256 = "73053bd1b3219e7012ec1f7ee0cc7bbb94a07bb44249d8f4002be0ba4bddbd4e"

def download_and_verify(url, expected_hash, dest_path):
    if not url.startswith("https://github.com/pact-foundation/"):
        print("Invalid URL domain!")
        sys.exit(1)
    if not re.fullmatch(r"[0-9a-f]{64}", expected_hash):
        print("Invalid expected hash format!")
        sys.exit(1)

    print(f"Downloading {url}...")
    req = urllib.request.Request(url, headers={'User-Agent': 'Mozilla/5.0'})
    try:
        # bounded network read
        with urllib.request.urlopen(req, timeout=30) as response:
            data = b""
            while True:
                chunk = response.read(8192)
                if not chunk:
                    break
                data += chunk
                if len(data) > 50 * 1024 * 1024:
                    print("File too large! Max 50MB allowed.")
                    sys.exit(1)
    except urllib.error.URLError as e:
        print(f"Failed to download: {e}")
        sys.exit(1)

    h = hashlib.sha256()
    h.update(data)
    actual_hash = h.hexdigest()

    if actual_hash != expected_hash:
        print(f"Hash mismatch for {url}!")
        print(f"Expected (SHA256): {expected_hash}")
        print(f"Actual   (SHA256): {actual_hash}")
        sys.exit(1)

    print(f"Hash verified (SHA256): {actual_hash}, Size: {len(data)} bytes")

    # Save to a temporary file instead of deleting on error
    tmp_path = dest_path + ".tmp"
    with open(tmp_path, "wb") as f:
        f.write(data)
    os.rename(tmp_path, dest_path)

def extract_standalone(tgz_path, dest_dir="."):
    print("Extracting pact-standalone...")
    if not hasattr(tarfile, 'data_filter'):
        print("tarfile.data_filter unavailable; refusing extraction (fail-closed)")
        sys.exit(1)
    with tarfile.open(tgz_path, "r:gz") as tar:
        try:
            tar.extractall(path=dest_dir, filter="data")
        except Exception as e:
            print(f"Safe extraction failed: {e}")
            sys.exit(1)

def extract_ffi(gz_path):
    home_dir = os.environ.get("HOME", ".")
    lib_dir = os.path.join(home_dir, ".pact", "lib")
    os.makedirs(lib_dir, exist_ok=True)
    ffi_so = os.path.join(lib_dir, "libpact_ffi.so")
    print(f"Extracting libpact_ffi to {ffi_so}...")
    with gzip.open(gz_path, "rb") as f_in:
        with open(ffi_so, "wb") as f_out:
            shutil.copyfileobj(f_in, f_out)

def main():
    parser = argparse.ArgumentParser(description="Download and verify Pact artifacts securely.")
    parser.add_argument("--type", choices=["standalone", "ffi"], required=True, help="Artifact type to download")
    args = parser.parse_args()

    os.makedirs("pact_download", exist_ok=True)

    if args.type == "standalone":
        standalone_tgz = "pact_download/pact-standalone.tar.gz"
        download_and_verify(PACT_STANDALONE_URL, PACT_STANDALONE_SHA256, standalone_tgz)
        extract_standalone(standalone_tgz)
    elif args.type == "ffi":
        ffi_gz = "pact_download/libpact_ffi.so.gz"
        download_and_verify(LIBPACT_FFI_URL, LIBPACT_FFI_SHA256, ffi_gz)
        extract_ffi(ffi_gz)

    print(f"Artifact '{args.type}' downloaded and verified successfully.")

if __name__ == "__main__":
    main()
