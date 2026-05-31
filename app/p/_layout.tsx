import { Slot } from 'expo-router'

// Public package routes live under /p/<slug>/. This layout intentionally has
// no auth gate — packages declaring `publicRoutes` in their manifest get a
// pre-auth entry point (e.g. drive's share links). Per-package subtrees are
// generator-owned; do not hand-edit anything under p/<slug>/.
export default function PublicPackagesLayout() {
    return <Slot />
}
