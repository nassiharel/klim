VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

MODULE  := github.com/nassiharel/clim
LDFLAGS := -s -w \
  -X $(MODULE)/internal/build.Version=$(VERSION) \
  -X $(MODULE)/internal/build.Commit=$(COMMIT) \
  -X $(MODULE)/internal/build.Date=$(DATE)

.PHONY: build run test lint clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/clim ./cmd/clim

run:
	go run -ldflags "$(LDFLAGS)" ./cmd/clim

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/ dist/
