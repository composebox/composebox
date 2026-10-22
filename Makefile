.PHONY: all build ui test clean run docker

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "v0.1.0")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -s -w -X main.Version=$(VERSION) -X main.GitCommit=$(COMMIT) -X main.BuildDate=$(DATE)

all: build

ui:
	cd ui && npm ci && npm run build

build: ui
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/composebox ./cmd/composebox

test:
	go vet ./...
	go test -v -race -count=1 ./...

run: build
	./bin/composebox -port 8080 -stacks ./stacks

docker:
	docker build -t composebox:latest .

clean:
	rm -rf bin/ ui/dist/index.html ui/dist/assets/
