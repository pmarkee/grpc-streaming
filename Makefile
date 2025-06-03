BUILD_DIR := bin

.PHONY: server
server:
	go build -o $(BUILD_DIR)/server ./cmd/server

.PHONY: client
client:
	go build -o $(BUILD_DIR)/client ./cmd/client

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

.PHONY: certs
certs:
	openssl genpkey -algorithm ED25519 -out certs/server.key
	openssl req -new -x509 -key certs/server.key -days 365 -out certs/server.crt -config server-cert-config.cnf -extensions v3_req

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make server    - Build the server executable"
	@echo "  make client    - Build the client executable"
	@echo "  make fmt       - Format the code"
	@echo "  make clean     - Clean the build artifacts"
	@echo "  make deps      - Install dependencies"
	@echo "  make generate  - Compile protobuf schemas"
