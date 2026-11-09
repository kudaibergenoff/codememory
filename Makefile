.PHONY: build test race vet fmt vuln check dist
VERSION ?= dev
build:
	CGO_ENABLED=0 go build -o bin/codememory ./cmd/codememory
test:
	go test ./...
race:
	go test -race ./...
vet:
	go vet ./...
fmt:
	test -z "$$(gofmt -l .)"
vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
check: fmt vet test race build
dist:
	scripts/dist.sh $(VERSION)
