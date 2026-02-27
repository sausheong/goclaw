BINARY    := goclaw
CMD       := ./cmd/goclaw
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS   := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: build build-app build-small run test test-race test-v lint fmt vet tidy clean snapshot install

## build: compile the binary
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)

## build-app: compile the menu bar app as a macOS .app bundle
build-app:
	go build -ldflags "$(LDFLAGS)" -o goclaw-app ./cmd/goclaw-app
	rm -rf GoClaw.app
	mkdir -p GoClaw.app/Contents/MacOS GoClaw.app/Contents/Resources
	cp goclaw-app GoClaw.app/Contents/MacOS/goclaw-app
	cp cmd/goclaw-app/Info.plist GoClaw.app/Contents/Info.plist
	cp cmd/goclaw-app/icon.icns GoClaw.app/Contents/Resources/icon.icns
	rm -f goclaw-app
	@echo "Built GoClaw.app"

## build-small: compile a smaller, statically-linked binary (+ UPX on Linux)
build-small:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)
	@if [ "$$(uname)" = "Linux" ] && command -v upx >/dev/null 2>&1; then \
		upx --best --lzma $(BINARY); \
	elif [ "$$(uname)" = "Linux" ]; then \
		echo "tip: install upx for further compression (apt install upx)"; \
	fi

## run: build and start the gateway
run: build
	./$(BINARY) start

## test: run all tests
test:
	go test ./...

## test-race: run tests with race detector
test-race:
	go test -race ./...

## test-v: run tests with verbose output
test-v:
	go test -v ./...

## lint: run golangci-lint
lint:
	golangci-lint run

## fmt: format all Go source files
fmt:
	go fmt ./...

## vet: run go vet
vet:
	go vet ./...

## tidy: tidy and verify module dependencies
tidy:
	go mod tidy
	go mod verify

## clean: remove build artifacts
clean:
	rm -f $(BINARY) goclaw-app
	rm -rf GoClaw.app
	go clean

## snapshot: cross-platform build via goreleaser
snapshot:
	goreleaser build --snapshot --clean

## install: install the binary to $GOPATH/bin
install:
	go install -ldflags "$(LDFLAGS)" $(CMD)

## help: show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
