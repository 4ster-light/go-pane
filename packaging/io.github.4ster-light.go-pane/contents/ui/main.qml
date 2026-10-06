import QtQuick
import org.kde.plasma.plasmoid
import org.kde.plasma.core as PlasmaCore
import "format.js" as Fmt

PlasmoidItem {
    id: root

    readonly property string daemonUrl: Plasmoid.configuration.daemonUrl
    readonly property int refreshMs: Math.max(5, Plasmoid.configuration.refreshSeconds) * 1000

    property var data: ({})
    property string lastError: ""
    property double lastUpdated: 0

    function refresh() {
        var xhr = new XMLHttpRequest();
        xhr.open("GET", root.daemonUrl + "/v1/usage");
        xhr.timeout = 10000;
        xhr.onreadystatechange = function () {
            if (xhr.readyState !== XMLHttpRequest.DONE) {
                return;
            }
            if (xhr.status === 200) {
                try {
                    root.data = JSON.parse(xhr.responseText);
                    root.lastError = "";
                    root.lastUpdated = Date.now();
                } catch (e) {
                    root.lastError = i18n("Invalid response from daemon");
                }
            } else {
                root.lastError = i18n("Daemon not reachable (HTTP %1)", xhr.status);
            }
        };
        xhr.ontimeout = function () { root.lastError = i18n("Daemon request timed out"); };
        xhr.onerror = function () { root.lastError = i18n("Daemon not reachable"); };
        xhr.send();
    }

    Timer {
        interval: root.refreshMs
        running: true
        repeat: true
        triggeredOnStart: true
        onTriggered: root.refresh()
    }

    toolTipMainText: i18n("OpenCode Go Usage")
    toolTipSubText: {
        var lines = [];
        var order = (root.data && root.data.order) ? root.data.order : ["rolling", "weekly", "monthly"];
        for (var i = 0; i < order.length; i++) {
            var m = root.data.windows ? root.data.windows[order[i]] : null;
            if (!m) {
                continue;
            }
            lines.push(m.title + ": " + Fmt.formatPercent(m.availablePercent) + " left, resets in "
                + Fmt.formatDuration(m.remainingSeconds));
        }
        if (root.lastError) {
            lines.push(root.lastError);
        }
        return lines.join("\n");
    }

    compactRepresentation: CompactRepresentation {
        data: root.data
        lastError: root.lastError
        warn: Plasmoid.configuration.warnPercent
        critical: Plasmoid.configuration.criticalPercent
        compactMode: Plasmoid.configuration.compactMode
    }

    fullRepresentation: FullRepresentation {
        data: root.data
        lastError: root.lastError
        lastUpdated: root.lastUpdated
        warn: Plasmoid.configuration.warnPercent
        critical: Plasmoid.configuration.criticalPercent
        showRolling: Plasmoid.configuration.showRolling
        showWeekly: Plasmoid.configuration.showWeekly
        showMonthly: Plasmoid.configuration.showMonthly
        onRefreshRequested: root.refresh()
    }
}
