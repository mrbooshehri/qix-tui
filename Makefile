BINARY ?= qix

.PHONY: build install run test vet fmt clean

build:
	go build -trimpath -o $(BINARY) .

install:
	go install .

run: build
	./$(BINARY)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './graphify-out/*')

clean:
	rm -f $(BINARY) coverage.out
