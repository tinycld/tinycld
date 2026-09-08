// Category keys, their display names, and their nav icons.
//
// The keys come from the generated table (which inherits them from
// emojibase); this module owns how they are ordered and named in the UI.

import {
    Apple,
    Clock,
    Flag,
    Hash,
    Lightbulb,
    type LucideIcon,
    Medal,
    Plane,
    Rabbit,
    Smile,
} from 'lucide-react-native'

/** Not a table category — assembled at runtime from the user's own history. */
export const FREQUENT_CATEGORY = 'frequent'

export const CATEGORY_ORDER = [
    FREQUENT_CATEGORY,
    'smileys_people',
    'animals_nature',
    'food_drink',
    'activities',
    'travel_places',
    'objects',
    'symbols',
    'flags',
] as const

export const CATEGORY_LABELS: Readonly<Record<string, string>> = {
    [FREQUENT_CATEGORY]: 'Frequently used',
    smileys_people: 'Smileys & people',
    animals_nature: 'Animals & nature',
    food_drink: 'Food & drink',
    activities: 'Activities',
    travel_places: 'Travel & places',
    objects: 'Objects',
    symbols: 'Symbols',
    flags: 'Flags',
}

export const CATEGORY_ICONS: Readonly<Record<string, LucideIcon>> = {
    [FREQUENT_CATEGORY]: Clock,
    smileys_people: Smile,
    animals_nature: Rabbit,
    food_drink: Apple,
    activities: Medal,
    travel_places: Plane,
    objects: Lightbulb,
    symbols: Hash,
    flags: Flag,
}
