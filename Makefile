all: test

test:
	test -z "$$(gofmt -s -l . | grep -v '^vendor' | tee /dev/stderr)"
	test -z "$$(golint $$(go list -mod=vendor ./... | grep -v /vendor/) | grep -v '/mtest/.*: should not use dot imports' | tee /dev/stderr)"
	CGO_ENABLED=0 GOLDFLAGS="-w -s" go install -mod=vendor ./pkg/...
	go test -mod=vendor -race -v ./...
	go vet -mod=vendor ./...

mod:
	go mod tidy
	go mod vendor
	git add -f vendor
	git add go.mod

.PHONY:	all test mod
