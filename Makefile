.PHONY: build test test-ui test-browser check demo clean
NODE ?= node

build:
	go build -o bin/gmeter ./cmd/gmeter
	go build -o bin/gmeter-demo ./cmd/gmeter-demo

test: test-ui
	go test -race ./...

test-ui:
	$(NODE) --check internal/dashboard/assets/theme.js
	$(NODE) --check internal/dashboard/assets/app.js
	$(NODE) --check internal/dashboard/assets/journey-editor.js
	$(NODE) --test internal/dashboard/test/*.test.cjs

test-browser: build
	$(NODE) internal/dashboard/test/journey-browser.cjs

check: test-ui
	go vet ./...
	go test -race ./...

demo:
	go run ./cmd/gmeter-demo

clean:
	rm -rf bin
