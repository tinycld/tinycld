import type { CustomKind } from './definition'
import type { FormHostDefInput } from './host'

// Pure data: the generator imports this in Node, so it imports no generated
// code. Core owns this host, so no package manifest declares it.
export const CORE_FORM_HOST_SLUG = 'core'
export const WEB_FORM_HOST_KEY = 'core:web_form'

// A web form is filled by a visitor with no session: a person field cannot
// read users, and the collaborative editors need a signed-in room.
export const WEB_FORM_KINDS: CustomKind[] = [
    'text',
    'textarea',
    'number',
    'date',
    'toggle',
    'select',
    'checklist',
    'file',
    'computed',
    'table',
]

export const CORE_FORM_HOSTS: FormHostDefInput[] = [
    {
        key: WEB_FORM_HOST_KEY,
        label: 'Web form',
        collection: 'form_submissions',
        formColumn: 'form',
        fieldsColumn: 'values',
        columns: [
            'id',
            'form',
            'publication',
            'values',
            'client_id',
            'ip_hash',
            'user_agent',
            'created',
        ],
        builtin: [],
        kinds: WEB_FORM_KINDS,
        files: {
            collection: 'form_submission_files',
            recordColumn: 'submission',
            fieldColumn: 'field',
            sizeColumn: 'size',
            uploaderColumn: 'uploaded_by',
        },
    },
]
