Version := $(shell git describe --tags --dirty)
GitCommit := $(shell git rev-parse HEAD)
LDFLAGS := "-s -w -X main.Version=$(Version) -X main.GitCommit=$(GitCommit)"
CONTAINERD_VER := 1.6.8
CNI_VERSION := v0.9.1
ARCH := amd64

export GO111MODULE=on

.PHONY: all
all: dist hashgen

.PHONY: publish
publish: dist hashgen

local:
	CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o bin/fecore

.PHONY: test
test:
	CGO_ENABLED=0 GOOS=linux go test -mod=vendor -ldflags $(LDFLAGS) ./...

.PHONY: dist
dist:
	CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags $(LDFLAGS) -a -installsuffix cgo -o bin/fecore

.PHONY: arm
arm:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -mod=vendor -ldflags $(LDFLAGS) -a -installsuffix cgo -o bin/fecore-armhf

.PHONY: arm64
arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -mod=vendor -ldflags $(LDFLAGS) -a -installsuffix cgo -o bin/fecore-arm64
  
.PHONY: hashgen
hashgen:
	for f in bin/fecore*; do shasum -a 256 $$f > $$f.sha256; done

dev:
	go build && \
	sudo mv fecore /usr/local/bin/fecore
