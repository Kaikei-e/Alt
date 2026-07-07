FROM postgres:18

RUN apt-get update && apt-get install -y \
  git \
  make \
  gcc \
  postgresql-server-dev-18 \
  clang \
  llvm \
  && rm -rf /var/lib/apt/lists/*

RUN git clone --branch v0.8.4 --depth 1 https://github.com/pgvector/pgvector.git \
  && cd pgvector \
  && [ "$(git rev-parse HEAD)" = "1d458ad5d73731241e8b912cacc2079beb643d30" ] \
  && make \
  && make install \
  && cd .. \
  && rm -rf pgvector

RUN apt-get remove -y git make gcc postgresql-server-dev-18 clang llvm \
  && apt-get autoremove -y
