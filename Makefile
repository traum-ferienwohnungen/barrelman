PROJECT_NAME := "barrelman"
PKG_LIST := $(shell go list | grep -v /vendor/)
MGOPATH := $(shell go env GOPATH)

.PHONY: all dep build clean test coverage coverhtml lint

all: build

lint: $(MGOPATH)/bin/golangci-lint ## Run lint
	@golangci-lint run

test: ## Run unittests
	@go test ${PKG_LIST}

race: dep ## Run data race detector
	@go test -race ${PKG_LIST}

msan: export CC=clang
msan: dep ## Run memory sanitizer
	@go test -msan ${PKG_LIST}

testall: test race msan ## Run all tests

coverage: ## Generate global code coverage report
	./coverage.sh;

coverhtml: ## Generate global code coverage report in HTML
	./coverage.sh html;

dep: ## Get the dependencies
	@go get -v -d ./...

$(MGOPATH)/bin/golangci-lint:
	@curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s -- -b $(MGOPATH)/bin v1.17.1

build: export CGO_ENABLED=0
build: export GOOS=linux
build: dep ## Build the binary file
	@go build -a -installsuffix cgo -o $(PROJECT_NAME) .

clean: ## Remove previous build
	@rm -f $(PROJECT_NAME) $(MGOPATH)/bin/golangci-lint

help: ## Display this help screen
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
