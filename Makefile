GORELEASER ?= goreleaser
TOFU ?= tofu
ZENSICAL ?= zensical

.PHONY: build test race vet check-agent-docs validate-missions smoke-test docker-integration docs-build docs-check docs-serve tofu-check release-check release-snapshot check check-all run clean

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

docs-build:
	@command -v $(ZENSICAL) >/dev/null 2>&1 || { echo "error: Zensical is required; install requirements-docs.txt in a Python 3.10+ virtual environment" >&2; exit 1; }
	$(ZENSICAL) build --clean

docs-check:
	@command -v $(ZENSICAL) >/dev/null 2>&1 || { echo "error: Zensical is required; install requirements-docs.txt in a Python 3.10+ virtual environment" >&2; exit 1; }
	$(ZENSICAL) build --clean --strict

docs-serve:
	@command -v $(ZENSICAL) >/dev/null 2>&1 || { echo "error: Zensical is required; install requirements-docs.txt in a Python 3.10+ virtual environment" >&2; exit 1; }
	$(ZENSICAL) serve

tofu-check:
	@command -v $(TOFU) >/dev/null 2>&1 || { echo "error: OpenTofu 1.12 is required; see infra/github/README.md" >&2; exit 1; }
	$(TOFU) -chdir=infra/github fmt -check -diff -recursive
	$(TOFU) -chdir=infra/github init -backend=false -input=false -lockfile=readonly -no-color
	$(TOFU) -chdir=infra/github validate -no-color

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
	rm -rf site
