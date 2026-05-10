.PHONY: run worker migrate build tidy

run:
	go run ./cmd/server

worker:
	go run ./cmd/worker

migrate:
	go run ./cmd/server --migrate-only

build:
	go build -o bin/server ./cmd/server
	go build -o bin/worker ./cmd/worker

tidy:
	go mod tidy
