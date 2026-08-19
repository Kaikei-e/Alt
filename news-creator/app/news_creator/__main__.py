"""python -m news_creator — production listener on :11434, not main.py's :8001."""

from news_creator.infra.inbound_server import main

if __name__ == "__main__":
    main()
