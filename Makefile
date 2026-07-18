GORELEASER ?= goreleaser

.PHONY: build test race vet check-agent-docs validate-missions smoke-test docker-integration release-check release-snapshot check check-all run clean

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

release-check:
	@command -v $(GORELEASER) >/dev/null 2>&1 || { echo "error: GoReleaser v2 is required; see https://goreleaser.com/install/" >&2; exit 1; }
	$(GORELEASER) check

release-snapshot: release-check
	$(GORELEASER) release --snapshot --clean

check: check-agent-docs test vet build smoke-test

check-all: check race

run:
	go run ./cmd/opsquest play

clean:
	go clean
	rm -f bin/opsquest
