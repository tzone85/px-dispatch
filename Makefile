BINARY := px
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/tzone85/px-dispatch/internal/cli.version=$(VERSION)"

.PHONY: build test lint clean install sync-vault watch-vault

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/px

test:
	go test ./... -race -coverprofile=coverage.out

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY) coverage.out

install: build
	cp $(BINARY) $(GOPATH)/bin/

# One-shot push of docs/obsidian/ into the local Obsidian vault.
# Override the destination with PX_DISPATCH_VAULT=/custom/path make sync-vault.
sync-vault:
	@./scripts/sync-obsidian-vault.sh

# fswatch-driven continuous sync; needs `brew install fswatch`.
watch-vault:
	@./scripts/sync-obsidian-vault.sh --watch
