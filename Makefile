.PHONY: all fast update lint test build coverage gomutants clean

all: update lint test build coverage gomutants clean

fast: lint test build coverage clean

update:
	@echo "==> Upgrading Go version and dependencies..."
	@echo "==> Updating tooling configurations from danrneal/go-tools..."
	curl -sSfL -z .golangci.yml https://raw.githubusercontent.com/danrneal/go-tools/main/.golangci.yml -o .golangci.yml
	curl -sSfL -z Makefile https://raw.githubusercontent.com/danrneal/go-tools/main/Makefile -o Makefile
	@echo "==> Installing latest CLI tools..."
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $$(go env GOPATH)/bin latest
	go install github.com/gomutants/gomutants@latest
	go install github.com/danrneal/go-tools/cmd/cover-diff@latest
	go install github.com/danrneal/go-tools/cmd/gomutants-ignore@latest
	$(eval GO_VERSION=$(shell $(shell go env GOPATH)/bin/golangci-lint version --json | jq -r '.goVersion | sub("^go"; "")'))
	go get go@$(GO_VERSION)
	go get -u ./...
	go mod tidy

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

gomutants: build
	@echo "==> Running gomutants..."
	$(shell go env GOPATH)/bin/gomutants --detect-equivalent --html-output=mutation-report.html ./...
	@echo "==> Filtering ignored mutations..."
	$(shell go env GOPATH)/bin/gomutants-ignore -htmlreport=mutation-report.html
	@echo "==> Moving report to Downloads..."
	mv mutation-report.html ~/Downloads/mutation-report.html
	rm -f mutation-report.json

clean:
	@echo "==> Cleaning up artifacts..."
	rm -f coverage.out mutation-report.json
