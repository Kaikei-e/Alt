# ---- Base image -----------------------------------------------------------
FROM ollama/ollama:latest

USER root

# ---- Install additional dependencies --------------------------------------
RUN apt-get update \
    && apt-get install --no-install-recommends -y \
    ca-certificates \
    curl \
    gosu \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# ---- Create ollama user --------------------------------------------------
RUN groupadd -f -g 2000 ollama-user \
    && useradd -u 2000 -g 2000 -m -s /bin/bash ollama-user || true

WORKDIR /home/ollama-user

# ---- Copy Modelfiles -----------------------------------------------------
COPY --chown=ollama-user:ollama-user Modelfile.gpt-oss20b-cpu /home/ollama-user/
COPY --chown=ollama-user:ollama-user Modelfile.gpt-oss20b-igpu /home/ollama-user/
COPY --chown=ollama-user:ollama-user Modelfile.qwen3-14b-rag /home/ollama-user/

# ---- Environment configuration -------------------------------------------
ENV OLLAMA_HOST=0.0.0.0:11434
ENV OLLAMA_ORIGINS=*
ENV OLLAMA_KEEP_ALIVE=-1
ENV OLLAMA_MAX_LOADED_MODELS=1
ENV OLLAMA_NUM_PARALLEL=1
ENV OLLAMA_MAX_QUEUE=1
ENV OLLAMA_FLASH_ATTENTION=1
ENV OLLAMA_KV_CACHE_TYPE=q8_0
ENV HOME=/home/ollama-user
ENV OLLAMA_HOME=/home/ollama-user/.ollama

# ---- Model cache persistence ---------------------------------------------
VOLUME ["/home/ollama-user/.ollama"]

# ---- Copy entrypoint and set permissions --------------------------------
COPY --chown=ollama-user:ollama-user entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# ---- Health check --------------------------------------------------------
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=5 \
    CMD curl -f http://localhost:11434/api/tags || exit 1

EXPOSE 11434

# ---- Runtime user (handled by entrypoint) --------------------------------
# USER ollama-user

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
