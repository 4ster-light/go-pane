#!/usr/bin/env python3
"""Static checks for the go-pane Plasma applet package.

This deliberately avoids a Qt/KDE toolchain so it can run anywhere:

* ``metadata.json`` is valid JSON with the fields Plasma needs.
* ``contents/config/main.xml`` is valid KConfigXT XML.
* every ``Plasmoid.configuration.<key>`` used in the QML is declared in
  ``main.xml`` (catches typos that would otherwise fail silently at runtime).
* the expected package layout exists.
"""

from __future__ import annotations

import json
import re
import sys
import xml.etree.ElementTree as ET
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
PACKAGE_DIR = REPO_ROOT / "packaging" / "io.github.4ster-light.go-pane"
MAIN_QML = PACKAGE_DIR / "contents" / "ui" / "main.qml"
MAIN_XML = PACKAGE_DIR / "contents" / "config" / "main.xml"
METADATA = PACKAGE_DIR / "metadata.json"

KCFG_NS = "{http://www.kde.org/standards/kcfg/1.0}"
CONFIG_REF = re.compile(r"Plasmoid\.configuration\.([A-Za-z_][A-Za-z0-9_]*)")


def fail(message: str) -> None:
    print(f"error: {message}", file=sys.stderr)
    raise SystemExit(1)


def check_layout() -> None:
    for path in (METADATA, MAIN_QML, MAIN_XML):
        if not path.is_file():
            fail(f"missing required file: {path.relative_to(REPO_ROOT)}")


def check_metadata() -> dict:
    try:
        data = json.loads(METADATA.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        fail(f"metadata.json is not valid JSON: {exc}")

    if data.get("KPackageStructure") != "Plasma/Applet":
        fail("metadata.json: KPackageStructure must be 'Plasma/Applet'")

    plugin = data.get("KPlugin")
    if not isinstance(plugin, dict):
        fail("metadata.json: missing KPlugin object")

    for field in ("Id", "Name", "Version", "License"):
        if not plugin.get(field):
            fail(f"metadata.json: KPlugin.{field} is required")

    version = plugin["Version"]
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        fail(f"metadata.json: Version {version!r} is not MAJOR.MINOR.PATCH")

    return data


def check_config_xml() -> set[str]:
    try:
        tree = ET.parse(MAIN_XML)
    except ET.ParseError as exc:
        fail(f"main.xml is not valid XML: {exc}")

    entries = set()
    for entry in tree.getroot().iter(f"{KCFG_NS}entry"):
        name = entry.get("name")
        if not name:
            fail("main.xml: <entry> without a name")
        entries.add(name)

    if not entries:
        fail("main.xml: no configuration entries found")
    return entries


def check_qml_config_refs(entries: set[str]) -> None:
    source = MAIN_QML.read_text(encoding="utf-8")
    if "PlasmoidItem" not in source:
        fail("main.qml: does not define a PlasmoidItem")

    used = set(CONFIG_REF.findall(source))
    undeclared = sorted(used - entries)
    if undeclared:
        fail(
            "main.qml references configuration keys that main.xml does not "
            f"declare: {', '.join(undeclared)}"
        )

    declared_re = re.compile(r"<entry name=\"([^\"]+)\"")
    declared = set(declared_re.findall(MAIN_XML.read_text(encoding="utf-8")))
    unused = sorted(declared - used)
    if unused:
        print(f"warning: declared but unused configuration keys: {', '.join(unused)}")


def main() -> None:
    check_layout()
    check_metadata()
    entries = check_config_xml()
    check_qml_config_refs(entries)
    print(f"ok: {PACKAGE_DIR.relative_to(REPO_ROOT)} is a valid Plasma applet package")


if __name__ == "__main__":
    main()
