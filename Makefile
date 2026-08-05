.PHONY: all build test clean fmt vet

BINARY_NAME=esc2ps

all: build

build:
	go build -o $(BINARY_NAME) .

test:
	go test -v ./...

clean:
	go clean
	rm -f $(BINARY_NAME)

fmt:
	go fmt ./...

vet:
	go vet ./...
