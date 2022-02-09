.PHONY: cover viewcover test lint

test:
	@go test -v ./...

cover:
	@go test -v -coverpkg=./... -coverprofile=coverage.out.tmp ./...

viewcover:
	go tool cover -html=coverage.out

lint:
	golangci-lint run ./...
