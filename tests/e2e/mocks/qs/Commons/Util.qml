pragma Singleton
import QtQuick
QtObject {
    function alpha(value, opacity) {
        return Qt.rgba(value.r, value.g, value.b, opacity)
    }
}
