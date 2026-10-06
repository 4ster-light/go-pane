// go-pane — a self-contained OpenCode Go usage widget for KDE Plasma 6.
//
// This single file authenticates against the OpenCode Go API, derives the
// rolling / weekly / monthly pacing metrics, and renders both the compact
// panel representation and the detailed popup. There is no daemon and no
// helper QML: everything lives here.
import QtQuick
import QtQuick.Layouts
import org.kde.plasma.plasmoid
import org.kde.plasma.core as PlasmaCore
import org.kde.plasma.components as PlasmaComponents3
import org.kde.kirigami as Kirigami

PlasmoidItem {
    id: root

    // --- Configuration ---------------------------------------------------
    // Defaults come from contents/config/main.xml.
    readonly property string baseUrl: {
        var v = Plasmoid.configuration.baseUrl;
        return (v && v.length > 0) ? v : "https://opencode.ai/zen/go/v1";
    }
    readonly property string apiKey: Plasmoid.configuration.apiKey || ""
    readonly property int refreshMs: Math.max(5, Plasmoid.configuration.refreshSeconds || 60) * 1000

    readonly property var windowOrder: ["rolling", "weekly", "monthly"]
    // Window lengths are inferred: rolling ~5h and the weekly reset is Monday
    // 00:00 UTC; monthly is a signup anniversary.
    readonly property var windowSeconds: ({
        rolling: 5 * 3600,
        weekly: 7 * 24 * 3600,
        monthly: 30 * 24 * 3600
    })

    // --- State -----------------------------------------------------------
    property var usage: ({ order: ["rolling", "weekly", "monthly"], headline: "", windows: {} })
    property string lastError: ""
    property double lastUpdated: 0
    property bool showSettings: false
    property bool keyVisible: false

    // --- Formatting helpers ---------------------------------------------
    function clamp(v, lo, hi) {
        return Math.max(lo, Math.min(hi, v));
    }

    function formatDuration(seconds) {
        if (seconds === undefined || seconds === null || isNaN(seconds) || seconds < 0) {
            return "0m";
        }
        var totalMinutes = Math.round(seconds / 60);
        var days = Math.floor(totalMinutes / 1440);
        var hours = Math.floor((totalMinutes % 1440) / 60);
        var mins = totalMinutes % 60;
        if (days > 0) {
            return i18n("%1d %2h", days, hours);
        }
        if (hours > 0) {
            return i18n("%1h %2m", hours, mins);
        }
        return i18n("%1m", mins);
    }

    function formatPercent(v) {
        if (v === undefined || v === null || isNaN(v)) {
            return "--";
        }
        if (v > 0 && v < 1) {
            return v.toFixed(1) + "%";
        }
        return Math.round(v) + "%";
    }

    function barColor(used) {
        if (used >= Plasmoid.configuration.criticalPercent) {
            return Kirigami.Theme.negativeTextColor;
        }
        if (used >= Plasmoid.configuration.warnPercent) {
            return Kirigami.Theme.neutralTextColor;
        }
        return Kirigami.Theme.positiveTextColor;
    }

    function paceLabel(pace) {
        if (pace === undefined || pace === null || isNaN(pace)) {
            return "";
        }
        if (pace >= 0.85 && pace <= 1.15) {
            return i18n("on pace");
        }
        return i18n("%1× pace", pace.toFixed(2));
    }

    function exhaustLabel(metric) {
        if (!metric || metric.exhaustsAt === null || metric.exhaustsAt === undefined) {
            return "";
        }
        if (!(metric.exhaustsAt < metric.resetsAt)) {
            return "";
        }
        var secs = (metric.exhaustsAt - Date.now()) / 1000;
        if (secs <= 0) {
            return i18n("empty now");
        }
        return i18n("empty in %1", root.formatDuration(secs));
    }

    // --- Metrics ---------------------------------------------------------
    function computeWindow(now, kind, raw, winSec) {
        var used = root.clamp(Number(raw && raw.percent) || 0, 0, 100);
        var resetsMs = Date.parse(raw && raw.resetsAt);
        if (isNaN(resetsMs)) {
            resetsMs = now;
        }
        var windowMs = Math.max(1, winSec) * 1000;
        var elapsedMs = root.clamp(now - (resetsMs - windowMs), 0, windowMs);
        var elapsedFrac = elapsedMs / windowMs;
        var remainingMs = Math.max(0, resetsMs - now);
        var usedFrac = used / 100;
        var pace = elapsedFrac > 0 ? usedFrac / elapsedFrac : null;
        var exhaustsAt = null;
        if (elapsedFrac > 0 && usedFrac > 0) {
            exhaustsAt = now + elapsedMs * (1 - usedFrac) / usedFrac;
        }
        return {
            kind: kind,
            title: kind.charAt(0).toUpperCase() + kind.slice(1),
            status: (raw && raw.status) ? raw.status : "",
            usedPercent: used,
            availablePercent: 100 - used,
            resetsAt: resetsMs,
            windowSeconds: winSec,
            elapsedPercent: root.clamp(elapsedFrac * 100, 0, 100),
            remainingSeconds: Math.floor(remainingMs / 1000),
            paceRatio: pace,
            projectedUsedPercent: pace === null ? null : pace * 100,
            exhaustsAt: exhaustsAt,
            safeRatePerHour: remainingMs > 0 ? (100 - used) / (remainingMs / 3600000) : 0,
            budgetPerHour: 100 / (windowMs / 3600000)
        };
    }

    function applyUsage(rawUsage) {
        var now = Date.now();
        var windows = {};
        var best = "";
        var bestScore = -Infinity;
        for (var i = 0; i < root.windowOrder.length; i++) {
            var kind = root.windowOrder[i];
            var m = root.computeWindow(now, kind, rawUsage ? rawUsage[kind] : null, root.windowSeconds[kind]);
            windows[kind] = m;
            var score = (m.projectedUsedPercent === null) ? m.usedPercent : m.projectedUsedPercent;
            if (score > bestScore) {
                bestScore = score;
                best = kind;
            }
        }
        root.usage = { order: root.windowOrder, headline: best, windows: windows };
    }

    // --- Fetching --------------------------------------------------------
    function refresh() {
        if (!root.apiKey) {
            root.lastError = i18n("No API key configured");
            return;
        }
        var xhr = new XMLHttpRequest();
        xhr.open("GET", root.baseUrl.replace(/\/+$/, "") + "/usage");
        xhr.timeout = 15000;
        xhr.setRequestHeader("Authorization", "Bearer " + root.apiKey);
        xhr.setRequestHeader("Accept", "application/json");
        xhr.onreadystatechange = function () {
            if (xhr.readyState !== XMLHttpRequest.DONE) {
                return;
            }
            if (xhr.status === 200) {
                try {
                    var payload = JSON.parse(xhr.responseText);
                    root.applyUsage(payload && payload.usage ? payload.usage : payload);
                    root.lastError = "";
                    root.lastUpdated = Date.now();
                } catch (e) {
                    root.lastError = i18n("Invalid response from the API");
                }
            } else if (xhr.status === 401 || xhr.status === 403) {
                root.lastError = i18n("API key rejected (HTTP %1)", xhr.status);
            } else if (xhr.status === 429) {
                root.lastError = i18n("Rate limited (HTTP 429)");
            } else if (xhr.status === 0) {
                root.lastError = i18n("API not reachable");
            } else {
                root.lastError = i18n("Request failed (HTTP %1)", xhr.status);
            }
        };
        xhr.ontimeout = function () { root.lastError = i18n("Request timed out"); };
        xhr.onerror = function () { root.lastError = i18n("API not reachable"); };
        xhr.send();
    }

    Component.onCompleted: {
        if (!root.apiKey) {
            root.showSettings = true;
        }
    }

    Timer {
        interval: root.refreshMs
        running: true
        repeat: true
        triggeredOnStart: true
        onTriggered: root.refresh()
    }

    // --- Tooltip ---------------------------------------------------------
    toolTipMainText: i18n("OpenCode Go Usage")
    toolTipSubText: {
        var lines = [];
        var windows = (root.usage && root.usage.windows) ? root.usage.windows : {};
        for (var i = 0; i < root.windowOrder.length; i++) {
            var m = windows[root.windowOrder[i]];
            if (!m) {
                continue;
            }
            lines.push(i18n("%1: %2 left, resets in %3",
                            m.title,
                            root.formatPercent(m.availablePercent),
                            root.formatDuration(m.remainingSeconds)));
        }
        if (root.lastError) {
            lines.push(root.lastError);
        }
        return lines.join("\n");
    }

    // --- Compact representation (panel) ----------------------------------
    compactRepresentation: Item {
        id: compact

        Layout.minimumWidth: implicitWidth
        Layout.minimumHeight: implicitHeight
        Layout.preferredWidth: implicitWidth
        Layout.preferredHeight: implicitHeight
        Layout.maximumWidth: implicitWidth
        Layout.maximumHeight: implicitHeight
        implicitWidth: compactLayout.implicitWidth + Kirigami.Units.smallSpacing * 2
        implicitHeight: compactLayout.implicitHeight + Kirigami.Units.smallSpacing * 2

        function headlineMetric() {
            var windows = (root.usage && root.usage.windows) ? root.usage.windows : null;
            if (!windows) {
                return null;
            }
            var mode = Plasmoid.configuration.compactMode;
            if (mode === "monthly" && windows.monthly) {
                return windows.monthly;
            }
            if (mode === "rolling" && windows.rolling) {
                return windows.rolling;
            }
            return windows[root.usage.headline] || windows.monthly || windows.weekly || windows.rolling || null;
        }

        ColumnLayout {
            id: compactLayout
            anchors.centerIn: parent
            spacing: 1

            PlasmaComponents3.Label {
                Layout.alignment: Qt.AlignHCenter
                text: {
                    var m = compact.headlineMetric();
                    if (!m) {
                        return root.lastError ? "!" : "\u2026";
                    }
                    return root.formatPercent(m.availablePercent);
                }
                font.bold: true
                font.pointSize: Kirigami.Theme.smallFont.pointSize
            }

            RowLayout {
                Layout.alignment: Qt.AlignHCenter
                spacing: 2

                Repeater {
                    model: root.windowOrder

                    delegate: Rectangle {
                        required property string modelData
                        readonly property var metric: (root.usage && root.usage.windows) ? root.usage.windows[modelData] : null

                        width: Math.max(2, Kirigami.Units.smallSpacing)
                        height: Kirigami.Units.gridUnit
                        radius: width / 2
                        color: Qt.rgba(Kirigami.Theme.textColor.r, Kirigami.Theme.textColor.g, Kirigami.Theme.textColor.b, 0.15)

                        Rectangle {
                            anchors.bottom: parent.bottom
                            width: parent.width
                            radius: parent.radius
                            height: parent.height * (parent.metric ? Math.min(1, Math.max(0, parent.metric.usedPercent / 100)) : 0)
                            color: parent.metric ? root.barColor(parent.metric.usedPercent) : Kirigami.Theme.disabledTextColor
                        }
                    }
                }
            }
        }
    }

    // --- Full representation (popup) -------------------------------------
    fullRepresentation: Item {
        id: full

        Layout.preferredWidth: Kirigami.Units.gridUnit * 22
        Layout.preferredHeight: column.implicitHeight + Kirigami.Units.largeSpacing * 2
        Layout.minimumWidth: Kirigami.Units.gridUnit * 18
        Layout.minimumHeight: column.implicitHeight + Kirigami.Units.largeSpacing * 2

        function visibleOrder() {
            var all = (root.usage && root.usage.order) ? root.usage.order : root.windowOrder;
            var cfg = Plasmoid.configuration;
            return all.filter(function (k) {
                if (k === "rolling") {
                    return cfg.showRolling;
                }
                if (k === "weekly") {
                    return cfg.showWeekly;
                }
                if (k === "monthly") {
                    return cfg.showMonthly;
                }
                return true;
            });
        }

        ColumnLayout {
            id: column
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.margins: Kirigami.Units.largeSpacing
            spacing: Kirigami.Units.smallSpacing

            RowLayout {
                Layout.fillWidth: true

                PlasmaComponents3.Label {
                    Layout.fillWidth: true
                    text: i18n("OpenCode Go")
                    font.bold: true
                }

                PlasmaComponents3.Label {
                    visible: root.lastUpdated > 0
                    text: root.lastUpdated > 0 ? i18n("updated %1", Qt.formatTime(new Date(root.lastUpdated), "hh:mm:ss")) : ""
                    font: Kirigami.Theme.smallFont
                    opacity: 0.7
                }

                PlasmaComponents3.ToolButton {
                    icon.name: "view-refresh"
                    onClicked: root.refresh()
                    PlasmaComponents3.ToolTip {
                        text: i18n("Refresh now")
                    }
                }

                PlasmaComponents3.ToolButton {
                    icon.name: "configure"
                    checkable: true
                    checked: root.showSettings
                    onClicked: root.showSettings = checked
                    PlasmaComponents3.ToolTip {
                        text: i18n("Settings")
                    }
                }
            }

            // Usage rows
            ColumnLayout {
                Layout.fillWidth: true
                visible: !root.showSettings
                spacing: Kirigami.Units.smallSpacing

                Repeater {
                    model: full.visibleOrder()

                    delegate: ColumnLayout {
                        id: rowItem
                        required property string modelData
                        Layout.fillWidth: true
                        spacing: Kirigami.Units.smallSpacing / 2

                        readonly property var metric: (root.usage && root.usage.windows) ? root.usage.windows[modelData] : null

                        RowLayout {
                            Layout.fillWidth: true
                            spacing: Kirigami.Units.smallSpacing

                            PlasmaComponents3.Label {
                                Layout.fillWidth: true
                                text: rowItem.metric ? rowItem.metric.title : ""
                                font.bold: true
                            }

                            PlasmaComponents3.Label {
                                text: rowItem.metric ? i18n("%1 left", root.formatPercent(rowItem.metric.availablePercent)) : "--"
                                font.bold: true
                                color: rowItem.metric ? root.barColor(rowItem.metric.usedPercent) : Kirigami.Theme.disabledTextColor
                            }
                        }

                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: Math.max(4, Kirigami.Units.smallSpacing)
                            radius: height / 2
                            color: Qt.rgba(Kirigami.Theme.textColor.r, Kirigami.Theme.textColor.g, Kirigami.Theme.textColor.b, 0.15)

                            Rectangle {
                                width: parent.width * Math.min(1, Math.max(0, (rowItem.metric ? rowItem.metric.usedPercent : 0) / 100))
                                height: parent.height
                                radius: parent.radius
                                color: rowItem.metric ? root.barColor(rowItem.metric.usedPercent) : Kirigami.Theme.disabledTextColor
                                Behavior on width {
                                    NumberAnimation { duration: 200; easing.type: Easing.OutCubic }
                                }
                            }
                        }

                        RowLayout {
                            Layout.fillWidth: true
                            spacing: Kirigami.Units.smallSpacing

                            PlasmaComponents3.Label {
                                text: rowItem.metric ? i18n("resets in %1", root.formatDuration(rowItem.metric.remainingSeconds)) : ""
                                font: Kirigami.Theme.smallFont
                                opacity: 0.8
                            }

                            Item { Layout.fillWidth: true }

                            PlasmaComponents3.Label {
                                visible: !!(rowItem.metric && rowItem.metric.paceRatio !== null)
                                text: rowItem.metric ? root.paceLabel(rowItem.metric.paceRatio) : ""
                                font: Kirigami.Theme.smallFont
                                opacity: 0.8
                            }

                            PlasmaComponents3.Label {
                                text: rowItem.metric ? root.exhaustLabel(rowItem.metric) : ""
                                visible: text !== ""
                                color: Kirigami.Theme.negativeTextColor
                                font: Kirigami.Theme.smallFont
                            }
                        }
                    }
                }

                PlasmaComponents3.Label {
                    Layout.fillWidth: true
                    visible: text !== ""
                    wrapMode: Text.WordWrap
                    font: Kirigami.Theme.smallFont
                    color: Kirigami.Theme.negativeTextColor
                    text: root.lastError
                }
            }

            // Settings
            ColumnLayout {
                Layout.fillWidth: true
                visible: root.showSettings
                spacing: Kirigami.Units.smallSpacing

                PlasmaComponents3.Label {
                    text: i18n("Connection")
                    font.bold: true
                }

                RowLayout {
                    Layout.fillWidth: true
                    spacing: Kirigami.Units.smallSpacing

                    PlasmaComponents3.Label {
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 6
                        text: i18n("API key")
                    }

                    PlasmaComponents3.TextField {
                        id: apiKeyField
                        Layout.fillWidth: true
                        echoMode: root.keyVisible ? TextInput.Normal : TextInput.Password
                        placeholderText: "oc_sk_…"
                        Component.onCompleted: text = root.apiKey
                        onEditingFinished: {
                            Plasmoid.configuration.apiKey = text;
                            root.refresh();
                        }
                    }

                    PlasmaComponents3.ToolButton {
                        text: root.keyVisible ? i18n("Hide") : i18n("Show")
                        onClicked: root.keyVisible = !root.keyVisible
                    }
                }

                RowLayout {
                    Layout.fillWidth: true
                    spacing: Kirigami.Units.smallSpacing

                    PlasmaComponents3.Label {
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 6
                        text: i18n("API base URL")
                    }

                    PlasmaComponents3.TextField {
                        id: baseUrlField
                        Layout.fillWidth: true
                        placeholderText: "https://opencode.ai/zen/go/v1"
                        Component.onCompleted: text = root.baseUrl
                        onEditingFinished: {
                            Plasmoid.configuration.baseUrl = text;
                            root.refresh();
                        }
                    }
                }

                RowLayout {
                    Layout.fillWidth: true
                    spacing: Kirigami.Units.smallSpacing

                    PlasmaComponents3.Label {
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 6
                        text: i18n("Refresh (s)")
                    }

                    PlasmaComponents3.SpinBox {
                        from: 5
                        to: 3600
                        value: Plasmoid.configuration.refreshSeconds
                        onValueModified: Plasmoid.configuration.refreshSeconds = value
                    }
                }

                PlasmaComponents3.Label {
                    text: i18n("Display")
                    font.bold: true
                }

                RowLayout {
                    Layout.fillWidth: true
                    spacing: Kirigami.Units.smallSpacing

                    PlasmaComponents3.Label {
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 6
                        text: i18n("Warn / critical")
                    }

                    PlasmaComponents3.SpinBox {
                        from: 1
                        to: 100
                        value: Plasmoid.configuration.warnPercent
                        onValueModified: Plasmoid.configuration.warnPercent = value
                    }

                    PlasmaComponents3.SpinBox {
                        from: 1
                        to: 100
                        value: Plasmoid.configuration.criticalPercent
                        onValueModified: Plasmoid.configuration.criticalPercent = value
                    }
                }

                RowLayout {
                    Layout.fillWidth: true
                    spacing: Kirigami.Units.smallSpacing

                    PlasmaComponents3.Label {
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 6
                        text: i18n("Show")
                    }

                    PlasmaComponents3.CheckBox {
                        text: i18n("Rolling")
                        checked: Plasmoid.configuration.showRolling
                        onClicked: Plasmoid.configuration.showRolling = checked
                    }

                    PlasmaComponents3.CheckBox {
                        text: i18n("Weekly")
                        checked: Plasmoid.configuration.showWeekly
                        onClicked: Plasmoid.configuration.showWeekly = checked
                    }

                    PlasmaComponents3.CheckBox {
                        text: i18n("Monthly")
                        checked: Plasmoid.configuration.showMonthly
                        onClicked: Plasmoid.configuration.showMonthly = checked
                    }
                }

                RowLayout {
                    Layout.fillWidth: true
                    spacing: Kirigami.Units.smallSpacing

                    PlasmaComponents3.Label {
                        Layout.preferredWidth: Kirigami.Units.gridUnit * 6
                        text: i18n("Panel headline")
                    }

                    PlasmaComponents3.ComboBox {
                        id: compactCombo
                        Layout.fillWidth: true
                        textRole: "text"
                        valueRole: "value"
                        model: [
                            { text: i18n("Most constrained"), value: "constrained" },
                            { text: i18n("Monthly"), value: "monthly" },
                            { text: i18n("Rolling"), value: "rolling" }
                        ]
                        Component.onCompleted: currentIndex = indexOfValue(Plasmoid.configuration.compactMode)
                        onActivated: Plasmoid.configuration.compactMode = currentValue
                    }
                }

                PlasmaComponents3.Label {
                    Layout.fillWidth: true
                    wrapMode: Text.WordWrap
                    font: Kirigami.Theme.smallFont
                    opacity: 0.7
                    text: i18n("The API key is stored in this widget's Plasma configuration (a 0600 file). Window lengths are estimated: rolling ≈ 5h, weekly 7d, monthly 30d.")
                }
            }
        }
    }
}
