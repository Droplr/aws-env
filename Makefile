.PHONY: build test snapshot

build:
	go build -trimpath -o dist/aws-env .

test:
	go vet ./...
	go test -race ./...

# Local dry run of the release build (needs goreleaser).
snapshot:
	goreleaser release --snapshot --clean --skip=sign
