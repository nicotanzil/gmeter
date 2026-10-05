.PHONY: build test check demo clean

build:
	go build -o bin/gmeter ./cmd/gmeter
	go build -o bin/gmeter-demo ./cmd/gmeter-demo

test:
	go test -race ./...

check:
	go vet ./...
	go test -race ./...

demo:
	go run ./cmd/gmeter-demo

clean:
	rm -rf bin
