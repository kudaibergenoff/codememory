.PHONY: build test race vet check
build:
	go build -tags sqlite_fts5 -o bin/codememory ./cmd/codememory
test:
	go test -tags sqlite_fts5 ./...
race:
	go test -race -tags sqlite_fts5 ./...
vet:
	go vet -tags sqlite_fts5 ./...
check: test vet race build
