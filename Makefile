.PHONY: run test fmt vet check help api-lint

# View all make commands
help:
	@echo "Available commands:"
	@echo "  make run       Start the Go server"
	@echo "  make test      Run automated tests"
	@echo "  make fmt       Format Go code"
	@echo "  make vet       Run static analysis"
	@echo "  make check     Run tests and static analysis"
	@echo "  make help      Show available commands"
	@echo "  make api-lint  Validate the OpenAPI specification"

# Read env and run program
run:
	@set -a; . ./.env; set +a; go run .

# Run automated tests
test:
	go test ./...

# Format entire module
fmt:
	go fmt ./...

# Vet entire module
vet:
	go vet ./...

# Run test and static analysis in one go
check: test vet

# Validate the OpenAPI contract without requiring a global CLI installation.
api-lint:
	npx --yes @redocly/cli@2.67.1 lint docs/openapi.yaml --extends=spec