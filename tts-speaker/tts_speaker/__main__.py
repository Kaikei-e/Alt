"""python -m tts_speaker — production listener."""

# Settings must fail before step-ca enrollment
import tts_speaker.main  # noqa: F401
from tts_speaker.infra.inbound_server import main

if __name__ == "__main__":
    main()
