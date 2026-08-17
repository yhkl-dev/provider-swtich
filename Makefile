BINARY := psw
PREFIX ?= /usr/local

.PHONY: all
all: build

.PHONY: build
build:
	go build -o $(BINARY) .

.PHONY: test
test:
	go test -race -cover ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: fmt
fmt:
	gofmt -l -w .

.PHONY: check
check: fmt vet test

.PHONY: install
install: build
	install -m 755 $(BINARY) $(PREFIX)/bin/$(BINARY)

.PHONY: clean
clean:
	rm -f $(BINARY)
