#!/bin/bash

echo "Building windows-amd64"
GOOS=windows GOARCH=amd64 go build -o build/sco-server-windows-amd64 server/cmd/cli/main.go

echo "Building windows-arm64"
GOOS=windows GOARCH=arm64 go build -o build/sco-server-windows-arm64 server/cmd/cli/main.go

echo "Building linux-amd64"
GOOS=linux GOARCH=amd64 go build -o build/sco-server-linux-amd64 server/cmd/cli/main.go

echo "Building linux-arm64"
GOOS=linux GOARCH=arm64 go build -o build/sco-server-linux-arm64 server/cmd/cli/main.go

echo "Building darwin-amd64"
GOOS=darwin GOARCH=amd64 go build -o build/sco-server-darwin-amd64 server/cmd/cli/main.go

echo "Building darwin-arm64"
GOOS=darwin GOARCH=arm64 go build -o build/sco-server-darwin-arm64 server/cmd/cli/main.go
