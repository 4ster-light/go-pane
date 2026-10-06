import QtQuick
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import org.kde.plasma.components as PlasmaComponents3
import "format.js" as Fmt

Item {
    id: compact

    property var usage: ({})
    property string lastError: ""
    property int warn: 70
    property int critical: 90
    property string compactMode: "constrained"

    Layout.minimumWidth: implicitWidth
    Layout.minimumHeight: implicitHeight
    Layout.preferredWidth: implicitWidth
    Layout.preferredHeight: implicitHeight
    Layout.maximumWidth: implicitWidth
    Layout.maximumHeight: implicitHeight
    implicitWidth: layout.implicitWidth + Kirigami.Units.smallSpacing * 2
    implicitHeight: layout.implicitHeight + Kirigami.Units.smallSpacing * 2

    function order() {
        return (usage && usage.order) ? usage.order : ["rolling", "weekly", "monthly"];
    }

    function headline() {
        if (!usage || !usage.windows) {
            return null;
        }
        if (compactMode === "monthly" && usage.windows.monthly) {
            return usage.windows.monthly;
        }
        if (compactMode === "rolling" && usage.windows.rolling) {
            return usage.windows.rolling;
        }
        return usage.windows[usage.headline] || usage.windows.monthly || usage.windows.weekly || usage.windows.rolling || null;
    }

    function barColor(m) {
        if (!m) {
            return Kirigami.Theme.disabledTextColor;
        }
        if (m.usedPercent >= critical) {
            return Kirigami.Theme.negativeTextColor;
        }
        if (m.usedPercent >= warn) {
            return Kirigami.Theme.neutralTextColor;
        }
        return Kirigami.Theme.positiveTextColor;
    }

    ColumnLayout {
        id: layout
        anchors.centerIn: parent
        spacing: 1

        PlasmaComponents3.Label {
            Layout.alignment: Qt.AlignHCenter
            text: {
                var m = compact.headline();
                if (!m) {
                    return compact.lastError ? "!" : "\u2026";
                }
                return Fmt.formatPercent(m.availablePercent);
            }
            font.bold: true
            font.pointSize: Kirigami.Theme.smallFont.pointSize
        }

        RowLayout {
            Layout.alignment: Qt.AlignHCenter
            spacing: 2

            Repeater {
                model: compact.order()

                delegate: Rectangle {
                    required property string modelData
                    readonly property var metric: (compact.usage && compact.usage.windows) ? compact.usage.windows[modelData] : null

                    width: Math.max(2, Kirigami.Units.smallSpacing)
                    height: Kirigami.Units.gridUnit
                    radius: width / 2
                    color: Qt.rgba(Kirigami.Theme.textColor.r, Kirigami.Theme.textColor.g, Kirigami.Theme.textColor.b, 0.15)

                    Rectangle {
                        anchors.bottom: parent.bottom
                        width: parent.width
                        radius: parent.radius
                        height: parent.height * (metric ? Math.min(1, Math.max(0, metric.usedPercent / 100)) : 0)
                        color: compact.barColor(metric)
                    }
                }
            }
        }
    }
}
