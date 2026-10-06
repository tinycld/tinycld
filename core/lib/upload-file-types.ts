/**
 * A file to put in a multipart upload. On web it is the browser `File` a picker
 * or drop returns; on native it is an expo-file-system `File`, which implements
 * Blob over a file on disk. Every upload path accepts this type, so a plain
 * object pointing at a URI — which is not a Blob — cannot reach one.
 */
export type UploadFile = Blob & { readonly name: string }
