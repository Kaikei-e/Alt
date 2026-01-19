#!/usr/bin/env python3
"""Alt システム健全性アナライザー メインエントリーポイント

Usage:
    uv run python -m alt_metrics analyze --hours 24
    uv run python -m alt_metrics analyze --lang ja --verbose
    uv run python -m alt_metrics validate
"""

from __future__ import annotations

import sys

from alt_metrics.cli import cmd_analyze, cmd_validate, create_parser


def main() -> int:
    """メインエントリーポイント"""
    parser = create_parser()
    args = parser.parse_args()

    if args.command == "analyze":
        return cmd_analyze(args)
    elif args.command == "validate":
        return cmd_validate(args)
    else:
        parser.print_help()
        return 0


if __name__ == "__main__":
    sys.exit(main())
