PLASMOID_ID  := io.github.4ster-light.go-pane
PACKAGE_DIR  := packaging/$(PLASMOID_ID)
DIST_DIR     := dist
VERSION      := $(shell sed -n 's/.*"Version": "\([^"]*\)".*/\1/p' $(PACKAGE_DIR)/metadata.json | head -1)

.PHONY: check test validate package plasmoid-install plasmoid-upgrade install uninstall dev

# Static checks that do not need a desktop session.
check: validate

test: validate

validate:
	python3 scripts/validate_package.py

# Build a distributable .plasmoid archive.
package: validate
	@mkdir -p $(DIST_DIR)
	@rm -f $(DIST_DIR)/go-pane-$(VERSION).plasmoid
	cd $(PACKAGE_DIR) && zip -rq $(CURDIR)/$(DIST_DIR)/go-pane-$(VERSION).plasmoid metadata.json contents
	@echo "Built $(DIST_DIR)/go-pane-$(VERSION).plasmoid"

plasmoid-install:
	kpackagetool6 -t Plasma/Applet -i $(PACKAGE_DIR)

plasmoid-upgrade:
	kpackagetool6 -t Plasma/Applet -u $(PACKAGE_DIR)

install: validate
	@kpackagetool6 -t Plasma/Applet -i $(PACKAGE_DIR) 2>/dev/null \
		|| kpackagetool6 -t Plasma/Applet -u $(PACKAGE_DIR)
	@echo
	@echo "Installed. Add the 'OpenCode Go Usage' widget and set your API key"
	@echo "from the widget's settings (gear icon in the popup)."

uninstall:
	-kpackagetool6 -t Plasma/Applet -r $(PLASMOID_ID) 2>/dev/null

dev: plasmoid-upgrade
	plasmawindowed $(PLASMOID_ID)
