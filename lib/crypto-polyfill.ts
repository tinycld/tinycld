import 'react-native-get-random-values'
import { uuidFromRandomValues } from '@tinycld/core/lib/uuid'

// react-native-get-random-values installs getRandomValues but not randomUUID,
// and Hermes has no built-in crypto.randomUUID. Install a self-contained,
// getRandomValues-based UUID so callers can rely on the standard Web Crypto API
// on every platform. See lib/uuid.ts for why we avoid expo-crypto's delegating
// web implementation here.
if (typeof crypto !== 'undefined' && !crypto.randomUUID) {
    crypto.randomUUID = uuidFromRandomValues as Crypto['randomUUID']
}
