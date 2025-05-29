BUILD_DIR := bin

.PHONY: server
server:
	go build -o $(BUILD_DIR)/server ./cmd/server

.PHONY: fmt
fmt:
	go fmt $(shell go list ./...)
	swag fmt

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)/*

.PHONY: deps
deps:
	go mod tidy

.PHONY: generate
generate:
	protoc -I=api/proto --go_out=api/proto --go-grpc_out=api/proto api/proto/*.proto

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make server    - Build the server executable"
	@echo "  make fmt       - Format the code"
	@echo "  make clean     - Clean the build artifacts"
	@echo "  make deps      - Install dependencies"
	@echo "  make generaet  - Compile protobuf schemas"