#!/usr/bin/env python3
"""検証スクリプトを実行するラッパー"""
import sys
import os

# パスを追加
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

# 検証スクリプトをインポートして実行
from compute_recap_coverage import main

if __name__ == '__main__':
    # --verifyフラグを追加
    sys.argv = [sys.argv[0], '--verify']
    main()
