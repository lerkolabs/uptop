# Contributing

## Development

```sh
go run ./cmd/uptop -demo        # starts with sample data
ssh -p 23234 localhost              # connect to TUI
```

## Tests

```sh
go test ./...              # unit tests
go test -race ./...        # race detector
golangci-lint run ./...    # linting
```

## Pull Requests

- Branch from `main`, PR back to `main`
- Conventional Commits for messages (`feat:`, `fix:`, `chore:`)
- Tests must pass, linter must be clean
- One logical change per PR
