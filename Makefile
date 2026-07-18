.PHONY: build test race vet check-agent-docs validate-missions smoke-test docker-integration check check-all run clean

build:
	mkdir -p bin
	go build -o bin/opsquest ./cmd/opsquest

check-agent-docs:
	./scripts/check-agent-docs.sh

validate-missions:
	./scripts/validate-missions.sh

test: validate-missions

race:
	go test -race ./...

vet:
	go vet ./...

smoke-test:
	./scripts/smoke-test.sh

docker-integration:
	OPSQUEST_DOCKER_TEST=1 go test ./internal/dockerlab -run Integration -count=1

check: check-agent-docs test vet build smoke-test

check-all: check race

run:
	go run ./cmd/opsquest play

clean:
	go clean
	rm -f bin/opsquest
