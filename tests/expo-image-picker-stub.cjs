'use strict'

// Stub for expo-image-picker in unit tests.
// Since SDK 57 the package imports `expo` itself, and `expo`'s entry installs
// the winter runtime, which require()s TypeScript siblings that a bare Node
// test environment cannot load. Unit tests never open the system picker, so
// every launch resolves as cancelled and every permission as denied.
const cancelled = async () => ({ canceled: true, assets: null })
const denied = async () => ({ granted: false, canAskAgain: false, status: 'denied', expires: 'never' })

module.exports = {
    launchImageLibraryAsync: cancelled,
    launchCameraAsync: cancelled,
    requestCameraPermissionsAsync: denied,
    requestMediaLibraryPermissionsAsync: denied,
    getCameraPermissionsAsync: denied,
    getMediaLibraryPermissionsAsync: denied,
}
