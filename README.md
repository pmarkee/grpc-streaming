# gRPC rate streaming

A small showcase of a gRPC server & client setup in Go for streaming mocked currency exchange rates. It is not meant to
be production ready or used as a  dependency in a real project, but rather as educational material or simply a reference
for getting started.

## Features

- 📡 Small example gRPC server showcasing server-side streaming
- 💻 Simple CLI-based client for demonstration
- 🔐 TLS encryption for secure communication
- 🛠️ Makefile to automate common tasks
- 🧪 Internal mock data source using Go channels
- 🧵 Lightweight in-process pub-sub system to deliver real-time updates to multiple clients
- 🛑 Graceful shutdown on both server and client
- 📋 Structured logging via zerolog

## How to use

### Pre-requisites

- Minimum for running: `go`, `docker`, `docker compose`, `make`
- Additionally for development: `protoc-gen-go`, `protoc-gen-go-grpc`, `protobuf`

### Running

1. clone the repo
2. install dependencies
3. `make certs`
4. run the server via `docker compose up -d --build`
5. run the client via one of the following, or see `-help` for instructions

```sh
go run ./cmd/client -from USD -to EUR
# OR
make client && ./bin/client -from USD -to EUR
```

### Development

#### Setup

1. install dependencies
2. `make deps`
3. `make certs`
4. `go run ./cmd/server` or `make server && bin/server` or `docker compose up -d server`
5. `go run ./cmd/client` (or use `grpcurl` or any gRPC client of your preference)

In case of a change to the schema (`/api/proto/*.proto`) boilerplate needs to be re-generated via `make generate`. The server and client
both may need a re-build and restart.

#### Project structure

The project loosely follows the [Standard Project Layout](https://github.com/golang-standards/project-layout).

- executables are found in `/cmd` (with `/bin` containing the built executables)
- `/api/proto` contains the gRPC proto files (in this case, just a  single one) and the generated server and client boilerplate in the `/api/proto/pb` directory
- `/internal` in this case only contains the mock data source
- `/certs` contains the locally generated self-signed x509 key and cert - this is not version controlled, generate via `make certs`
