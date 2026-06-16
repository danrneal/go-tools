.PHONY: all fast update lint test build coverage gremlins go-mutesting clean

all: update lint test build coverage gremlins go-mutesting clean

fast: lint test build coverage clean

update:
	@echo "==> Upgrading Go version..."
	go get go@latest
	go mod tidy
	@echo "==> Updating tooling configurations from danrneal/go-tools..."
	curl -sSfL -z .golangci.yml https://raw.githubusercontent.com/danrneal/go-tools/main/.golangci.yml -o .golangci.yml
	curl -sSfL -z Makefile https://raw.githubusercontent.com/danrneal/go-tools/main/Makefile -o Makefile
	@echo "==> Installing latest CLI tools..."
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $$(go env GOPATH)/bin latest
	go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
	go install github.com/avito-tech/go-mutesting/cmd/go-mutesting@latest
	go install github.com/danrneal/go-tools/cmd/cover-diff@latest
	go install github.com/danrneal/go-tools/cmd/go-mutesting-ignore@latest

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

gremlins: build
	@echo "==> Running gremlins..."
	$(shell go env GOPATH)/bin/gremlins unleash --timeout-coefficient 25 --invert-assignments --invert-bitwise --invert-bwassign --invert-negatives --invert-logical --invert-loopctrl --remove-self-assignments -S lv

go-mutesting: test build
	@echo "==> Running go-mutesting (filtered)..."
	$(shell go env GOPATH)/bin/go-mutesting-ignore -coverprofile=coverage.out
	@echo "==> Moving report to Downloads..."
	mv go-mutesting-report.html ~/Downloads/go-mutesting-report.html

clean:
	@echo "==> Cleaning up artifacts..."
	rm -f coverage.out
