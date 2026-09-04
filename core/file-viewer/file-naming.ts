/**
 * Filename → mime, and PocketBase's stored name → a human one.
 *
 * Both are needed by any surface that lists PB `file` fields: PocketBase
 * stores no mime alongside the blob and mangles the name on the way in, so a
 * client holding only a record has to recover both. Mail derived these first;
 * they live here so boards is not a second copy.
 */

const EXTENSION_TO_MIME: Record<string, string> = {
    // images
    jpg: 'image/jpeg',
    jpeg: 'image/jpeg',
    png: 'image/png',
    gif: 'image/gif',
    webp: 'image/webp',
    svg: 'image/svg+xml',
    bmp: 'image/bmp',
    heic: 'image/heic',
    // documents
    pdf: 'application/pdf',
    txt: 'text/plain',
    md: 'text/plain',
    csv: 'text/csv',
    json: 'application/json',
    html: 'text/html',
    htm: 'text/html',
    css: 'text/css',
    js: 'text/javascript',
    xml: 'application/xml',
    // office
    doc: 'application/msword',
    docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    xls: 'application/vnd.ms-excel',
    xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    ppt: 'application/vnd.ms-powerpoint',
    pptx: 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
    // archives
    zip: 'application/zip',
    gz: 'application/gzip',
    tar: 'application/x-tar',
    // media
    mp3: 'audio/mpeg',
    wav: 'audio/wav',
    mp4: 'video/mp4',
    mov: 'video/quicktime',
    webm: 'video/webm',
}

export function mimeFromFilename(filename: string): string {
    const ext = filename.split('.').pop()?.toLowerCase() ?? ''
    return EXTENSION_TO_MIME[ext] ?? 'application/octet-stream'
}

/**
 * PocketBase stores an upload as `{name}_{random10}.{ext}`. Strip the suffix
 * for display — but only when it matches exactly, so a file the user really
 * did name `report_2024final.pdf` is left alone.
 */
export function cleanFilename(filename: string): string {
    const match = filename.match(/^(.+)_[a-zA-Z0-9]{10}(\.\w+)$/)
    return match ? match[1] + match[2] : filename
}

/** Whether this name is something the preview layer can draw inline. */
export function isImageFilename(filename: string): boolean {
    return mimeFromFilename(filename).startsWith('image/')
}
