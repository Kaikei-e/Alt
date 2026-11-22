.PHONY: generate test test-race test-cover bench

generate:
	@echo "Generating Server code..."
	@mkdir -p internal/adapter/rag_http/openapi
	@go tool -modfile=../scripts/codegen/go.tool.mod oapi-codegen -package openapi -generate types,echo5-server,spec spec/openapi.yaml > internal/adapter/rag_http/openapi/server.gen.go
	@echo "Generating Client code for alt-backend..."
	@mkdir -p ../alt-backend/app/orchestrator/gateway/rag_gateway
	@go tool -modfile=../scripts/codegen/go.tool.mod oapi-codegen -package rag_gateway -generate types,client -exclude-operation-ids CandidateSimilarity spec/openapi.yaml > ../alt-backend/app/orchestrator/gateway/rag_gateway/client.gen.go
	@echo "Done."

test:
	go test ./...

test-race:
	go test -race ./...

test-cover:
	go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

bench:
	go test -bench=. -benchmem ./internal/domain/... ./internal/usecase/...
