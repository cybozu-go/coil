all: test

test:
	test -z "$$(gofmt -s -l . | tee /dev/stderr)"
	test -z "$$(golint $$(go list ./... | grep -v /vendor/) | grep -v '/mtest/.*: should not use dot imports' | grep -v 'model/etcd.go.*: exported func NewTestEtcdModel returns unexported type model.etcdModel, which can be annoying to use' | tee /dev/stderr)"
	test -z "$$(nilerr ./... 2>&1 | tee /dev/stderr)"
	test -z "$$(custom-checker -restrictpkg.packages=html/template,log ./... 2>&1 | tee /dev/stderr)"
	ineffassign .
	go build ./...
	go test -race -v ./...
	go vet ./...

mod:
	go mod tidy
	git add go.mod

.PHONY:	all test mod
