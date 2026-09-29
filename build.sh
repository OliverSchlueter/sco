#!/bin/bash

# sco-server
echo "Building sco-server for windows-amd64"
GOOS=windows GOARCH=amd64 go build -o build/sco-server-windows-amd64 server/cmd/cli/main.go

echo "Building sco-server for windows-arm64"
GOOS=windows GOARCH=arm64 go build -o build/sco-server-windows-arm64 server/cmd/cli/main.go

echo "Building sco-server for linux-amd64"
GOOS=linux GOARCH=amd64 go build -o build/sco-server-linux-amd64 server/cmd/cli/main.go

echo "Building sco-server for linux-arm64"
GOOS=linux GOARCH=arm64 go build -o build/sco-server-linux-arm64 server/cmd/cli/main.go

echo "Building sco-server for darwin-amd64"
GOOS=darwin GOARCH=amd64 go build -o build/sco-server-darwin-amd64 server/cmd/cli/main.go

echo "Building sco-server for darwin-arm64"
GOOS=darwin GOARCH=arm64 go build -o build/sco-server-darwin-arm64 server/cmd/cli/main.go


# sco-agent
echo "Building sco-agent for windows-amd64"
GOOS=windows GOARCH=amd64 go build -o build/sco-agent-windows-amd64 agent/cmd/app/main.go

echo "Building sco-agent for windows-arm64"
GOOS=windows GOARCH=arm64 go build -o build/sco-agent-windows-arm64 agent/cmd/app/main.go

echo "Building sco-agent for linux-amd64"
GOOS=linux GOARCH=amd64 go build -o build/sco-agent-linux-amd64 agent/cmd/app/main.go

echo "Building sco-agent for linux-arm64"
GOOS=linux GOARCH=arm64 go build -o build/sco-agent-linux-arm64 agent/cmd/app/main.go

echo "Building sco-agent for darwin-amd64"
GOOS=darwin GOARCH=amd64 go build -o build/sco-agent-darwin-amd64 agent/cmd/app/main.go

echo "Building sco-agent for darwin-arm64"
GOOS=darwin GOARCH=arm64 go build -o build/sco-agent-darwin-arm64 agent/cmd/app/main.go