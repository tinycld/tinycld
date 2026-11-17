import { z } from '@tinycld/core/ui/form'

// The server's rule (usernames.Valid): 1-32 characters, lowercase letters,
// digits, dash or underscore. The deleted- prefix belongs to deleted users.
export const usernameField = z
    .string()
    .trim()
    .toLowerCase()
    .regex(
        /^[a-z0-9][a-z0-9_-]{0,31}$/,
        'Use 1-32 characters: lowercase letters, digits, dash or underscore'
    )
    .refine(name => !name.startsWith('deleted-'), 'Usernames that start with deleted- are reserved')

export const renameSchema = z.object({ username: usernameField })
export type RenameFormValues = z.infer<typeof renameSchema>
