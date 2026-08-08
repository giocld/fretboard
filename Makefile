# fretboard — build & test targets
#
#   make build      build ./fretboard from cmd/fretboard
#   make test       run the full internal test suite
#   make e2e        run the e2e module (tests/)
#   make gp-parser  build the Rust Guitar Pro parser
#   make install    install the binary to /usr/local/bin
#   make clean      remove the built binary

GO      ?= go
BINARY  := fretboard
PREFIX  ?= /usr/local

.PHONY: build test e2e gp-parser install clean

build:
	$(GO) build -o $(BINARY) ./cmd/fretboard

test:
	$(GO) test ./...

e2e:
	cd tests && $(GO) test ./...

gp-parser:
	cd tools/gp-parser && cargo build --release

install: build
	install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)

clean:
	rm -f $(BINARY)
