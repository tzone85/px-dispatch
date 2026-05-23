# Contributing to px-dispatch

## Development Setup

1. Clone the repo
2. Install Go 1.22+
3. Install dependencies: `go mod download`
4. Run tests: `make test`
5. Build: `make build`

## Code Style

- Follow Go conventions (gofmt, go vet)
- Keep files focused: <400 lines typical, <800 max
- Functions <50 lines
- Immutable patterns: return new objects, don't mutate
- Interfaces at package boundaries

## Testing

- TDD: write tests before implementation
- `go test ./... -race` must pass
- Target 80%+ coverage on core packages

## Pull Requests

1. Create a feature branch
2. Write tests first
3. Implement
4. Run `make lint && make test`
5. Submit PR with clear description

## Architecture

See docs/superpowers/specs/ for the design spec.
