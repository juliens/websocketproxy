.PHONY: test check

default: check test

test:
	GO111MODULE=on go test -v ./...

check:
	GO111MODULE=on go mod vendor
	golangci-lint run

