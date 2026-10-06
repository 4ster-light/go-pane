.pragma library

// Pure formatting helpers shared by the plasmoid's QML files.

function formatDuration(seconds) {
    if (seconds === undefined || seconds === null || isNaN(seconds) || seconds < 0) {
        return "0m";
    }
    var totalMinutes = Math.round(seconds / 60);
    var days = Math.floor(totalMinutes / 1440);
    var hours = Math.floor((totalMinutes % 1440) / 60);
    var mins = totalMinutes % 60;
    if (days > 0) {
        return days + "d " + hours + "h";
    }
    if (hours > 0) {
        return hours + "h " + mins + "m";
    }
    return mins + "m";
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

// paceLabel renders a pace ratio. The English words are supplied by the QML
// caller (via i18n) because .pragma library scripts cannot translate.
function paceLabel(pace, onPace, suffix) {
    if (pace === undefined || pace === null || isNaN(pace)) {
        return "";
    }
    if (pace >= 0.85 && pace <= 1.15) {
        return onPace;
    }
    return pace.toFixed(2) + suffix;
}

function exhaustLabel(metric, emptyNow, inPrefix) {
    if (!metric || !metric.exhaustsAt) {
        return "";
    }
    var reset = new Date(metric.resetsAt).getTime();
    var exhaust = new Date(metric.exhaustsAt).getTime();
    if (!(exhaust < reset)) {
        return "";
    }
    var secs = (exhaust - Date.now()) / 1000;
    if (secs <= 0) {
        return emptyNow;
    }
    return inPrefix + " " + formatDuration(secs);
}
