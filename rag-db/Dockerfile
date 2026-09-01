FROM postgres:18.6

# Build tools + pgvector compile + purge must share one RUN so the
# removed packages never land in a persisted layer (image-size).
RUN apt-get update && apt-get install -y --no-install-recommends \
  ca-certificates \
  git \
  make \
  gcc \
  postgresql-server-dev-18 \
  clang \
  llvm \
  && git clone --branch v0.8.6 --depth 1 https://github.com/pgvector/pgvector.git \
  && cd pgvector \
  && [ "$(git rev-parse HEAD)" = "8ee86c96f0fd72390f890aa8a336fda6d3ab4c6c" ] \
  && make \
  && make install \
  && cd .. \
  && rm -rf pgvector \
  && apt-get remove -y git make gcc postgresql-server-dev-18 clang llvm \
  && apt-get autoremove -y \
  && rm -rf /var/lib/apt/lists/*
