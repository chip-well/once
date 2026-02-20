.PHONY: build test bench lint

test:
	go test ./...

bench:
	go test -bench=. -benchmem ./...

lint:
	golangci-lint run

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out
