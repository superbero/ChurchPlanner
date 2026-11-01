.PHONY: tidy run build test docker-up docker-down

tidy:
	go mod tidy

run:
	go run ./cmd/server

build:
	CGO_ENABLED=0 go build -o bin/server ./cmd/server

test:
	go test ./...

docker-up:
	cd deploy && cp -n .env.example .env && docker compose up --build

docker-down:
	cd deploy && docker compose down
