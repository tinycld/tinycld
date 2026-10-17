import { qrPath } from '@tinycld/core/components/settings/security/qr-matrix'
import { View } from 'react-native'
import Svg, { Path } from 'react-native-svg'

// Always dark modules on a light card, in both themes: many scanners cannot
// read an inverted (light on dark) code. That is why this uses fixed black
// and white, not theme colors.
const QUIET_ZONE = 4

export function QrCode({
    value,
    size = 184,
    accessibilityLabel = 'QR code for your authenticator app',
}: {
    value: string
    size?: number
    accessibilityLabel?: string
}) {
    const { path, size: modules } = qrPath(value)
    const box = modules + QUIET_ZONE * 2
    return (
        <View
            className="self-center rounded-lg bg-white p-1"
            accessibilityLabel={accessibilityLabel}
        >
            <Svg width={size} height={size} viewBox={`${-QUIET_ZONE} ${-QUIET_ZONE} ${box} ${box}`}>
                <Path d={path} fill="black" />
            </Svg>
        </View>
    )
}
