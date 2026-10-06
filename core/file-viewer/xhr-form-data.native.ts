import { type FormDataEntries, formDataEntries } from '@tinycld/core/lib/streamed-upload'
import { File } from 'expo-file-system'

// React Native's XMLHttpRequest sends a file part only in its own
// `{ uri, name, type }` form, which its native networking reads from disk; an
// expo-file-system File appended as it is would not be sent. This is the one
// place that form is built, right where the body is handed to React Native.

export interface ReactNativeFilePart {
    uri: string
    name: string
    type: string
}

export type XhrFormPart = string | Blob | ReactNativeFilePart

export function toXhrParts(entries: FormDataEntries): [string, XhrFormPart][] {
    const parts: [string, XhrFormPart][] = []
    for (const [key, value] of entries) {
        if (value instanceof File) {
            parts.push([key, { uri: value.uri, name: value.name, type: value.type }])
        } else if (typeof value === 'string' || value instanceof Blob) {
            parts.push([key, value])
        }
    }
    return parts
}

export function toXhrFormData(formData: FormData): FormData {
    const entries = formDataEntries(formData)
    if (!entries.some(([, value]) => value instanceof File)) return formData
    const converted = new FormData()
    for (const [key, value] of toXhrParts(entries)) converted.append(key, value)
    return converted
}
