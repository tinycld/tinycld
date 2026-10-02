import type { ReactNode } from 'react'
import { View } from 'react-native'

export function SectionCard({ children }: { children: ReactNode }) {
    return (
        <View className="rounded-xl border p-4 bg-surface-secondary border-border">{children}</View>
    )
}
