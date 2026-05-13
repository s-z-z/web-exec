.PHONY: build run test vet clean fmt lint

BINARY   := web-exec
CMD      := ./cmd/web-exec
DATA_DIR := data
ADDR     := :8080
TIMEOUT  := 30s
GUN_TOKEN := s-z-z-web-exec-2026

build:
	go build -o $(BINARY) $(CMD)

run: build
	./$(BINARY) -addr $(ADDR) -data $(DATA_DIR) -timeout $(TIMEOUT) -token $(GUN_TOKEN)

dev:
	go run $(CMD) -addr $(ADDR) -data $(DATA_DIR) -timeout $(TIMEOUT) -token $(GUN_TOKEN)

test:
	go test ./internal/... -timeout 60s -v

vet:
	go vet ./...

fmt:
	gofmt -w ./internal/ ./cmd/

lint: vet fmt

clean:
	rm -f $(BINARY) $(BINARY).exe
	rm -rf $(DATA_DIR)