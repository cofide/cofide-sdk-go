test:
    go run gotest.tools/gotestsum@latest --format github-actions ./...

lint *args:
    golangci-lint run --show-stats {{args}}
