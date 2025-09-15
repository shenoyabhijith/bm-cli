#!/bin/bash
echo "Building bookmark CLI..."
go build -o bin/bookmark cmd/bookmark/main.go
echo "Build complete! Run with: ./bin/bookmark"



