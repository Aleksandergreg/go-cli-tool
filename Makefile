.PHONY: build test race vet check run clean

build:
	go build -o bin/opsquest ./cmd/opsquest

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

check: test vet

run:
	go run ./cmd/opsquest play

clean:
	go clean
	rm -f bin/opsquest
