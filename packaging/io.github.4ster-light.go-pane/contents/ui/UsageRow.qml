import QtQuick
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import org.kde.plasma.components as PlasmaComponents3
import "format.js" as Fmt

ColumnLayout {
    id: row

    property var metric: null
    property int warn: 70
    property int critical: 90

    spacing: Kirigami.Units.smallSpacing / 2

    readonly property real used: metric ? metric.usedPercent : 0
    readonly property color barColor: used >= critical ? Kirigami.Theme.negativeTextColor
                                      : used >= warn ? Kirigami.Theme.neutralTextColor
                                      : Kirigami.Theme.positiveTextColor

    RowLayout {
        Layout.fillWidth: true
        spacing: Kirigami.Units.smallSpacing

        PlasmaComponents3.Label {
            Layout.fillWidth: true
            text: row.metric ? row.metric.title : ""
            font.bold: true
        }

        PlasmaComponents3.Label {
            text: row.metric ? i18n("%1 left", Fmt.formatPercent(row.metric.availablePercent)) : "--"
            font.bold: true
            color: row.barColor
        }
    }

    Rectangle {
        Layout.fillWidth: true
        implicitHeight: Math.max(4, Kirigami.Units.smallSpacing)
        radius: height / 2
        color: Qt.rgba(Kirigami.Theme.textColor.r, Kirigami.Theme.textColor.g, Kirigami.Theme.textColor.b, 0.15)

        Rectangle {
            width: parent.width * Math.min(1, Math.max(0, row.used / 100))
            height: parent.height
            radius: parent.radius
            color: row.barColor
            Behavior on width {
                NumberAnimation { duration: 200; easing.type: Easing.OutCubic }
            }
        }
    }

    RowLayout {
        Layout.fillWidth: true
        spacing: Kirigami.Units.smallSpacing

        PlasmaComponents3.Label {
            text: row.metric ? i18n("resets in %1", Fmt.formatDuration(row.metric.remainingSeconds)) : ""
            font: Kirigami.Theme.smallFont
            opacity: 0.8
        }

        Item { Layout.fillWidth: true }

        PlasmaComponents3.Label {
            visible: row.metric && row.metric.paceRatio !== null && row.metric.paceRatio !== undefined
            text: row.metric ? Fmt.paceLabel(row.metric.paceRatio, i18n("on pace"), i18n("× pace")) : ""
            font: Kirigami.Theme.smallFont
            opacity: 0.8
        }

        PlasmaComponents3.Label {
            text: row.metric ? Fmt.exhaustLabel(row.metric, i18n("empty now"), i18n("empty in")) : ""
            visible: text !== ""
            color: Kirigami.Theme.negativeTextColor
            font: Kirigami.Theme.smallFont
        }
    }
}
