BINARY      := go-pane
PREFIX      ?= $(HOME)/.local
BINDIR      := $(PREFIX)/bin
UNITDIR     := $(HOME)/.config/systemd/user
PLASMOID_ID := io.github.4ster-light.go-pane
PLASMOID_DIR:= packaging/$(PLASMOID_ID)

VERSION := $(shell sed -n 's/.*"Version": "\([^"]*\)".*/\1/p' $(PLASMOID_DIR)/metadata.json | head -1)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/4ster-light/go-pane/internal/version.Version=$(VERSION) \
           -X github.com/4ster-light/go-pane/internal/version.Commit=$(COMMIT) \
           -X github.com/4ster-light/go-pane/internal/version.Date=$(DATE)

.PHONY: build test vet fmt run plasmoid-install plasmoid-upgrade install uninstall dev clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/go-pane

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

run:
	go run ./cmd/go-pane serve

plasmoid-install:
	kpackagetool6 -t Plasma/Applet -i $(PLASMOID_DIR)

plasmoid-upgrade:
	kpackagetool6 -t Plasma/Applet -u $(PLASMOID_DIR)

install: build
	install -Dm755 bin/$(BINARY) $(BINDIR)/$(BINARY)
	install -Dm644 packaging/go-pane.service $(UNITDIR)/go-pane.service
	@kpackagetool6 -t Plasma/Applet -i $(PLASMOID_DIR) 2>/dev/null \
		|| kpackagetool6 -t Plasma/Applet -u $(PLASMOID_DIR)
	@echo
	@echo "Installed. Enable the daemon with:"
	@echo "  systemctl --user daemon-reload && systemctl --user enable --now go-pane.service"

uninstall:
	-systemctl --user disable --now go-pane.service 2>/dev/null
	rm -f $(BINDIR)/$(BINARY) $(UNITDIR)/go-pane.service
	-kpackagetool6 -t Plasma/Applet -r $(PLASMOID_ID) 2>/dev/null

dev: plasmoid-upgrade
	plasmawindowed $(PLASMOID_ID)

clean:
	rm -rf bin
