import { forwardRef } from 'react'
import { Platform, StyleSheet, TextInput, type TextInputProps } from 'react-native'

/**
 * A thin wrapper around RN TextInput that removes the browser focus ring on web
 * and ensures consistent minimum height. Use this instead of raw TextInput
 * anywhere a non-form-controlled text input is needed.
 */
export const PlainInput = forwardRef<TextInput, TextInputProps>(function PlainInput(props, ref) {
    const { style, ...rest } = props
    return <TextInput ref={ref} style={[styles.base, style]} {...rest} />
})

const styles = StyleSheet.create({
    base: {
        minHeight: 28,
        ...(Platform.OS === 'web' ? ({ outlineStyle: 'none' } as Record<string, unknown>) : {}),
    },
})
