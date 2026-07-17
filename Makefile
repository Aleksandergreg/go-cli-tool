.PHONY: build test vet run clean

build:
	go build -o bin/opsquest ./cmd/opsquest

test:
	go test ./...

vet:
	go vet ./...

run:
	go run ./cmd/opsquest play

clean:
	go clean

