#!/usr/bin/env python3
"""検証スクリプトを実行するラッパー"""

import sys
from pathlib import Path

# パスを追加
sys.path.insert(0, str(Path(__file__).absolute().parent))

# 検証スクリプトをインポートして実行
from compute_recap_coverage import main  # noqa: I001 -- sys.path updated above to locate sibling module

if __name__ == "__main__":
    # Prepend --verify while preserving any user-supplied args.
    sys.argv = [sys.argv[0], "--verify", *sys.argv[1:]]
    main()
