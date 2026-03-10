.PHONY: build test clean fmt lint

build:
	CGO_ENABLED=0 go build -o mailfeed ./cmd/mailfeed

test:
	go test ./...

fmt:
	gofmt -w .

lint:
	go vet ./...

clean:
	rm -f mailfeed
