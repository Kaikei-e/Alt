.PHONY: generate

generate:
	@echo "Generating Server code..."
	@mkdir -p internal/adapter/http/openapi
	@go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest -package openapi -generate types,server,spec spec/openapi.yaml > internal/adapter/http/openapi/server.gen.go
	@echo "Generating Client code for alt-backend..."
	@mkdir -p ../alt-backend/app/gateway/rag_gateway
	@go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest -package rag_gateway -generate types,client spec/openapi.yaml > ../alt-backend/app/gateway/rag_gateway/client.gen.go
	@echo "Done."
