// Side-effect import: installs globalThis.crypto.getRandomValues on RN.
// No-op on web (the browser already provides it).
import 'react-native-get-random-values'
import { uuidFromRandomValues } from '@tinycld/core/lib/uuid'

// react-native-get-random-values doesn't install randomUUID, and Hermes has no
// built-in crypto.randomUUID. @tanstack/db's collection constructor calls
// crypto.randomUUID() at module init, so it must exist on every platform.
//
// We assign a self-contained getRandomValues-based UUID rather than
// expo-crypto's randomUUID: expo-crypto's *web* implementation delegates back
// to globalThis.crypto.randomUUID, so installing it as that property creates
// infinite recursion the first time it's called. That happens on web served
// over a non-secure origin (plain http on a LAN IP/hostname), where the
// browser exposes crypto.getRandomValues but not crypto.randomUUID — the guard
// passes, the delegating shim installs, and the next randomUUID() call blows
// the stack. uuidFromRandomValues depends only on getRandomValues, so it is
// safe everywhere.
const cryptoGlobal = (globalThis as { crypto?: Partial<Crypto> }).crypto
if (cryptoGlobal && !cryptoGlobal.randomUUID) {
    cryptoGlobal.randomUUID = uuidFromRandomValues as Crypto['randomUUID']
}
