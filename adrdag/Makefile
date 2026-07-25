.PHONY: build test vet parity corpus-check check-all clean

build:
	go build -o bin/adrdag .

test:
	go test ./...

vet:
	go vet ./...

# Living parity contract: diff adrdag against python3 scripts/adr_graph.py
# on the real docs/ADR corpus (check / resolve / graph).
parity:
	go test -tags parity ./cmd -run TestParity

# Structural invariants over the real corpus (no magic-number counts).
corpus-check:
	go test -tags corpus ./cmd -run TestRealCorpus

check-all: vet test corpus-check parity

clean:
	rm -rf bin
