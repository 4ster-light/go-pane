import QtQuick
import QtQuick.Controls as QQC2
import org.kde.kcmutils as KCM
import org.kde.kirigami as Kirigami

KCM.SimpleKCM {
    property alias cfg_daemonUrl: daemonUrl.text
    property alias cfg_refreshSeconds: refresh.value
    property alias cfg_warnPercent: warn.value
    property alias cfg_criticalPercent: critical.value
    property alias cfg_showRolling: showRolling.checked
    property alias cfg_showWeekly: showWeekly.checked
    property alias cfg_showMonthly: showMonthly.checked
    property alias cfg_compactMode: compactMode.currentValue

    Kirigami.FormLayout {
        QQC2.TextField {
            id: daemonUrl
            Kirigami.FormData.label: i18n("Daemon URL:")
            placeholderText: "http://127.0.0.1:17873"
        }

        QQC2.SpinBox {
            id: refresh
            from: 5
            to: 3600
            Kirigami.FormData.label: i18n("Refresh (seconds):")
        }

        QQC2.SpinBox {
            id: warn
            from: 1
            to: 100
            Kirigami.FormData.label: i18n("Warning at (%):")
        }

        QQC2.SpinBox {
            id: critical
            from: 1
            to: 100
            Kirigami.FormData.label: i18n("Critical at (%):")
        }

        QQC2.CheckBox {
            id: showRolling
            text: i18n("Show rolling window")
        }

        QQC2.CheckBox {
            id: showWeekly
            text: i18n("Show weekly window")
        }

        QQC2.CheckBox {
            id: showMonthly
            text: i18n("Show monthly window")
        }

        QQC2.ComboBox {
            id: compactMode
            textRole: "text"
            valueRole: "value"
            model: [
                { text: i18n("Most constrained"), value: "constrained" },
                { text: i18n("Monthly"), value: "monthly" },
                { text: i18n("Rolling"), value: "rolling" }
            ]
            Kirigami.FormData.label: i18n("Compact headline:")
        }
    }
}
