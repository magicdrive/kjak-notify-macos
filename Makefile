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
# Hephaestus for the release targets (release.yml pins v1.5.0)
HEPHAESTUS   ?= hephaestus
# staticcheck pinned as in kjak (2026.2.2, verified with Go 1.27.3)
STATICCHECK  := honnef.co/go/tools/cmd/staticcheck@v0.8.2

.PHONY: all build test race vet staticcheck fmt-check kjak integration fuzz check install uninstall clean \
	release-check snapshot homebrew-e2e

all: check build

build:
	$(GO) build -trimpath -o build/$(BIN) ./cmd/$(BIN)

# unit tests: terminal-notifier is a stub; nothing is posted
test:
	$(GO) test -count=1 . ./internal/... ./docs/

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

kjak: $(KJAK_BIN)

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

# Hephaestus: both configurations, and the release plan of a stable and a
# prerelease tag (a prerelease never updates the tap)
release-check:
	$(HEPHAESTUS) check
	$(HEPHAESTUS) check --config hephaestus.rehearsal.yaml
	$(HEPHAESTUS) publish plan v0.1.0 | grep -qx 'tap=publish'
	$(HEPHAESTUS) publish plan v0.1.0-rc.1 | grep -qx 'tap=skip'

# build, archive and checksum without a tag, as CI does (dist/)
snapshot:
	$(HEPHAESTUS) run --stages build,archive,checksum --snapshot --fresh
	scripts/verify-dist.sh dist

# the release formula with this Mac's Homebrew (installs into its prefix,
# then removes what it installed): scripts/homebrew-e2e.sh
homebrew-e2e: $(KJAK_BIN)
	HEPHAESTUS=$(HEPHAESTUS) KJAK_BIN=$(KJAK_BIN) scripts/homebrew-e2e.sh

clean:
	rm -rf build dist
