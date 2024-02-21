Version := $(shell date "+%Y%m%d%H%M")
GitCommit := $(shell git rev-parse HEAD)
DIR := $(shell pwd)
LDFLAGS := -s -w -X main.Version=$(Version) -X main.GitCommit=$(GitCommit) -X main.DEBUG=true

run:
	go run cmd/main.go --conf config.local.yaml --listen :8080

build:
	go build -race -ldflags "$(LDFLAGS)" -o build/debug/aidea-chat-server cmd/main.go

build-release:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/release/aidea-chat-server-linux cmd/main.go
	GOOS=linux GOARCH=arm go build -ldflags "$(LDFLAGS)" -o build/release/aidea-chat-server-linux-arm cmd/main.go
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/release/aidea-chat-server-darwin cmd/main.go
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/release/aidea-chat-server.exe cmd/main.go

build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/release/aidea-chat-server-linux cmd/main.go

orm:
	# https://github.com/mylxsw/eloquent
	eloquent gen --source 'pkg/repo/model/*.yaml'
	gofmt -s -w pkg/repo/model/*.go

.PHONY: build build-release orm build-linux
