import { packageIcons } from '@tinycld/app-generated/package-icons'
import { CircleHelp, type LucideIcon } from 'lucide-react-native'

export function getIcon(name: string): LucideIcon {
    return packageIcons[name] ?? CircleHelp
}
