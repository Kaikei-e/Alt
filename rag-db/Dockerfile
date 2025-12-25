FROM postgres:18

RUN apt-get update && apt-get install -y \
  git \
  make \
  gcc \
  postgresql-server-dev-18 \
  clang \
  llvm \
  && rm -rf /var/lib/apt/lists/*

RUN git clone https://github.com/pgvector/pgvector.git \
  && cd pgvector \
  && make \
  && make install \
  && cd .. \
  && rm -rf pgvector

RUN apt-get remove -y git make gcc postgresql-server-dev-18 clang llvm \
  && apt-get autoremove -y
