import { requireNativeModule } from 'expo'
import type { AppUpdaterModuleType } from './src/AppUpdater.types'

// Throws at import if the native module is absent (e.g. web/dev) — callers must
// guard on Platform.OS !== 'web' && !__DEV__.
export default requireNativeModule<AppUpdaterModuleType>('AppUpdaterModule')
