import QtQuick
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import org.kde.plasma.components as PlasmaComponents3

Item {
    id: full

    property var usage: ({})
    property string lastError: ""
    property double lastUpdated: 0
    property int warn: 70
    property int critical: 90
    property bool showRolling: true
    property bool showWeekly: true
    property bool showMonthly: true

    signal refreshRequested()

    Layout.preferredWidth: Kirigami.Units.gridUnit * 20
    Layout.preferredHeight: column.implicitHeight + Kirigami.Units.largeSpacing * 2
    Layout.minimumWidth: Kirigami.Units.gridUnit * 16
    Layout.minimumHeight: column.implicitHeight + Kirigami.Units.largeSpacing * 2

    function order() {
        var all = (usage && usage.order) ? usage.order : ["rolling", "weekly", "monthly"];
        return all.filter(function (k) {
            if (k === "rolling") {
                return full.showRolling;
            }
            if (k === "weekly") {
                return full.showWeekly;
            }
            if (k === "monthly") {
                return full.showMonthly;
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
                visible: full.lastUpdated > 0
                text: full.lastUpdated > 0 ? i18n("updated %1", Qt.formatTime(new Date(full.lastUpdated), "hh:mm:ss")) : ""
                font: Kirigami.Theme.smallFont
                opacity: 0.7
            }

            PlasmaComponents3.ToolButton {
                icon.name: "view-refresh"
                onClicked: full.refreshRequested()
            }
        }

        Repeater {
            model: full.order()

            delegate: UsageRow {
                required property string modelData
                Layout.fillWidth: true
                metric: (full.usage && full.usage.windows) ? full.usage.windows[modelData] : null
                warn: full.warn
                critical: full.critical
            }
        }

        PlasmaComponents3.Label {
            Layout.fillWidth: true
            visible: text !== ""
            wrapMode: Text.WordWrap
            font: Kirigami.Theme.smallFont
            color: Kirigami.Theme.negativeTextColor
            text: {
                if (full.usage && full.usage.stale && full.usage.error) {
                    return i18n("Stale: %1", full.usage.error);
                }
                return full.lastError;
            }
        }
    }
}
