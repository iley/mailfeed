.PHONY: build build.linux test clean fmt lint docker

build:
	CGO_ENABLED=0 go build -o mailfeed ./cmd/mailfeed

build.linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o mailfeed.linux ./cmd/mailfeed

test:
	go test ./...

fmt:
	gofmt -w .

lint:
	go vet ./...

docker:
	docker build -t mailfeed .

clean:
	rm -f mailfeed mailfeed.linux
