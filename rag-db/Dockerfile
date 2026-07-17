FROM postgres:18

# Build tools + pgvector compile + purge must share one RUN so the
# removed packages never land in a persisted layer (image-size).
RUN apt-get update && apt-get install -y --no-install-recommends \
  git \
  make \
  gcc \
  postgresql-server-dev-18 \
  clang \
  llvm \
  && git clone --branch v0.8.4 --depth 1 https://github.com/pgvector/pgvector.git \
  && cd pgvector \
  && [ "$(git rev-parse HEAD)" = "1d458ad5d73731241e8b912cacc2079beb643d30" ] \
  && make \
  && make install \
  && cd .. \
  && rm -rf pgvector \
  && apt-get remove -y git make gcc postgresql-server-dev-18 clang llvm \
  && apt-get autoremove -y \
  && rm -rf /var/lib/apt/lists/*
