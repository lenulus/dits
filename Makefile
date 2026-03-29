.PHONY: build build-cli build-server test vet fmt clean

BIN_DIR := bin

build: build-cli build-server

build-cli:
	go build -o $(BIN_DIR)/dits ./cmd/dits

build-server:
	go build -o $(BIN_DIR)/dits-server ./cmd/dits-server

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

clean:
	rm -rf $(BIN_DIR)
