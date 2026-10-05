pragma Singleton
import QtQuick
QtObject {
    readonly property int cornerRadius: 14
    readonly property QtObject font: QtObject {
        readonly property int caption: 12
        readonly property int bodySmall: 13
        readonly property int body: 15
        readonly property int title: 20
        readonly property int heading: 24
    }
    function space(value) { return value }
}
