.PHONY: run test fmt vet check help

# View all make commands
help:
	@echo "Available commands:"
	@echo "  make run       Start the Go server"
	@echo "  make test      Run automated tests"
	@echo "  make fmt       Format Go code"
	@echo "  make vet       Run static analysis"
	@echo "  make check     Run tests and static analysis"
	@echo "  make help      Show available commands"

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