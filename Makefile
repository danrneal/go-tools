.PHONY: all fast update lint test build coverage mutation_tests

all: update lint test build coverage mutation_tests

fast: lint test build coverage

update:
	@echo "==> Updating tooling configurations from danrneal/go-tools..."
	curl -sSfL https://raw.githubusercontent.com/danrneal/go-tools/main/.golangci.yml -o .golangci.yml
	curl -sSfL https://raw.githubusercontent.com/danrneal/go-tools/main/Makefile -o Makefile
	@echo "==> Installing latest CLI tools..."
	go install github.com/danrneal/go-tools/cmd/cover-diff@latest

lint:
	@echo "==> Running golangci-lint..."
	$(shell go env GOPATH)/bin/golangci-lint fmt
	$(shell go env GOPATH)/bin/golangci-lint run

test:
	@echo "==> Running tests..."
	go test -coverprofile=coverage.out ./...

build:
	@echo "==> Building binary..."
	go build -o bin/ ./cmd/...

coverage: test
	@echo "==> Generating HTML report..."
	go tool cover -html=coverage.out -o ~/Downloads/coverage.html
	@echo "==> Checking coverage diff..."
	$(shell go env GOPATH)/bin/cover-diff -coverprofile=coverage.out
	@echo "==> Cleaning up coverage.out..."
	rm -f coverage.out

mutation_tests: build
	@echo "==> Running gremlins..."
	$(shell go env GOPATH)/bin/gremlins unleash --timeout-coefficient 25 --invert-assignments --invert-bitwise --invert-bwassign --invert-negatives --invert-logical --invert-loopctrl --remove-self-assignments -S lv; GREMLINS_CODE=$$?; \
	echo "==> Running go-mutesting (filtered)..."; \
	go run tools/mutant_filter/main.go -base main -target ./... -out ~/Downloads/filtered-report.html; \
	echo "==> Checking mutation results..."; \
	if [ $$GREMLINS_CODE -ne 0 ]; then \
		echo "FAILURE: gremlins found surviving mutants. See output above."; \
		exit 1; \
	fi
