#!/bin/bash

set -x -e

# Lint.
go get github.com/golang/lint/golint
for package in $(go list ./... | grep -v '/vendor/'); do golint -set_exit_status $package; done

# Vet.
go vet $(go list ./... | grep -v '/vendor/')

# Test.
go test $(go list ./... | grep -v '/vendor/')
