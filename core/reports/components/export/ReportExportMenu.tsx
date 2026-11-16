import type { ToolbarItem } from '@tinycld/core/components/ResponsiveToolbar'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Menu } from '@tinycld/core/ui/menu'
import { ChevronDown } from 'lucide-react-native'
import type { ReportExportModel } from '../../hooks/useReportExport'
import { EXPORT_FORMATS } from '../../lib/export-client'
import { REPORT_TAP_TARGET } from '../ReportHeader'

const PREPARING = 'Preparing…'
const EXPORT = 'Export'

/**
 * The export rows: one download per format. The Export button's menu holds them,
 * and on a narrow toolbar "⋯" lists them as its own entries.
 */
export function ExportMenuRows({ model }: { model: ReportExportModel }) {
    const rows = EXPORT_FORMATS.map(entry => (
        <Menu.Item
            key={entry.format}
            testID={`report-export-${entry.format}`}
            label={entry.menuLabel}
            isDisabled={model.isDisabled}
            onSelect={() => model.onDownload(entry.format)}
        />
    ))
    return <>{rows}</>
}

/** In "⋯" a rule sets the export rows apart from the toolbar's other entries. */
function ExportOverflowRows({ model }: { model: ReportExportModel }) {
    return (
        <>
            <Menu.Separator />
            <ExportMenuRows model={model} />
        </>
    )
}

function ExportMenu({ model }: { model: ReportExportModel }) {
    const label = model.isPreparing ? PREPARING : EXPORT
    const ink = useThemeColor('foreground')
    // The chevron tells the reader that Export opens a menu, as Copy link does not.
    const trigger = (
        <Button
            testID="report-export-open"
            variant="outline"
            style={{ minHeight: REPORT_TAP_TARGET }}
            isDisabled={model.isDisabled}
        >
            <ButtonText>{label}</ButtonText>
            <ChevronDown size={16} color={ink} />
        </Button>
    )
    return (
        <Menu placement="bottom-end" title={EXPORT} trigger={trigger}>
            <ExportMenuRows model={model} />
        </Menu>
    )
}

/** The toolbar's Export item, after Copy link; folded, its rows are entries of "⋯". */
export function exportToolbarItem(model: ReportExportModel): ToolbarItem {
    return {
        type: 'custom',
        key: 'export',
        overflow: { rows: <ExportOverflowRows model={model} /> },
        element: <ExportMenu model={model} />,
    }
}
