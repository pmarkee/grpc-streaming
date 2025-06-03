# --- Build stage ---
FROM golang:1.24-alpine AS builder
RUN apk add make
WORKDIR /build
RUN mkdir /build/bin
COPY . .
RUN make deps
RUN make server

# --- Run stage ---
FROM alpine:3.21
WORKDIR /app
COPY --from=builder /build/bin/server .
EXPOSE 8080
ENTRYPOINT ["./server"]
