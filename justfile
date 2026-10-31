set dotenv-load := false

default:
    @just --list

fmt:
    gofmt -w cmd/crumbcli/*.go

fmt-check:
    test -z "$(gofmt -l cmd/crumbcli/*.go)"

vet:
    go vet ./...

test:
    go test -race -timeout 60s ./...

vuln:
    go tool govulncheck ./...

build:
    go build -trimpath -o bin/crumbcli ./cmd/crumbcli

e2e: build
    python3 scripts/e2e.py --binary bin/crumbcli --output artifacts/e2e.json

ci: fmt-check vet test vuln e2e
    go mod verify

dist:
    bash scripts/dist.sh

hooks:
    git config --local core.hooksPath .githooks
