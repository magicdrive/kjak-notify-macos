# kjak-notify-macos: kjak-feather-notify-macos, a kjak external feather
# plugin (protocol v1) posting macOS notifications with terminal-notifier.

GO      ?= go
BIN     := kjak-feather-notify-macos
PREFIX  ?= $(HOME)/.local
BINDIR  ?= $(PREFIX)/bin
# kjak for the integration tests (make integration): v1.1.0, built into
# build/ unless KJAK_BIN names an existing one
KJAK_VERSION ?= v1.1.0
KJAK_BIN     ?= $(CURDIR)/build/kjak
FUZZTIME     ?= 30s
# staticcheck pinned as in kjak (2026.2.2, verified with Go 1.27.3)
STATICCHECK  := honnef.co/go/tools/cmd/staticcheck@v0.8.2

.PHONY: all build test race vet staticcheck fmt-check integration fuzz check install uninstall clean

all: check build

build:
	$(GO) build -trimpath -o build/$(BIN) ./cmd/$(BIN)

# unit tests: terminal-notifier is a stub; nothing is posted
test:
	$(GO) test -count=1 ./internal/... ./docs/

race:
	$(GO) test -count=1 -race ./internal/...

vet:
	$(GO) vet ./...

staticcheck:
	$(GO) run $(STATICCHECK) ./...

fmt-check:
	@out=$$(gofmt -l .); [ -z "$$out" ] || { echo "gofmt: $$out"; exit 1; }

# kjak $(KJAK_VERSION) runs the plugin (stub terminal-notifier): nothing is posted
$(KJAK_BIN):
	GOBIN=$(abspath $(dir $(KJAK_BIN))) $(GO) install github.com/magicdrive/kjak@$(KJAK_VERSION)

integration: $(KJAK_BIN)
	KJAK_BIN=$(KJAK_BIN) $(GO) test -count=1 -v ./tests/

fuzz:
	$(GO) test -run '^$$' -fuzz '^FuzzArgs$$' -fuzztime $(FUZZTIME) ./internal/plugin/

check: fmt-check vet staticcheck test race integration

# into $(BINDIR) (an absolute directory of PATH; not writable by group or
# others, as kjak requires)
install: build
	install -d -m 755 $(BINDIR)
	install -m 755 build/$(BIN) $(BINDIR)/$(BIN)

uninstall:
	rm -f $(BINDIR)/$(BIN)

clean:
	rm -rf build
