.PHONY: tidy run build test test-race test-cover vet docker-up docker-down

tidy:
	go mod tidy

run:
	go run ./cmd/server

build:
	CGO_ENABLED=0 go build -o bin/server ./cmd/server

test:
	go test ./...

test-race:
	go test -race ./...

test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

vet:
	go vet ./...

docker-up:
	cd deploy && cp -n .env.example .env && docker compose up --build

docker-down:
	cd deploy && docker compose down
