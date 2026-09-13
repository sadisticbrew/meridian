.PHONY: build test vet fmt clean

build:
	go build -o ./bin/meridian ./cmd/meridian

test:
	go test ./...

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }

clean:
	rm -rf ./bin
