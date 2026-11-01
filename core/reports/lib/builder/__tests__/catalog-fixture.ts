import { catalogOf } from '../columns'

// The salesstub sources (tests/scripts/reports-stub/server/register.go.txt), as
// the catalog endpoint answers for them.
export const SALES_KEY = 'salesstub.sales'
export const TEAMS_KEY = 'salesstub.teams'

export const CATALOG = catalogOf([
    {
        key: SALES_KEY,
        label: 'Stub sales',
        fields: [
            { key: 'id', label: 'ID', type: 'relation', aggregates: ['count', 'countDistinct'] },
            {
                key: 'customer',
                label: 'Customer',
                type: 'text',
                aggregates: ['count', 'countDistinct'],
            },
            {
                key: 'amount',
                label: 'Amount',
                type: 'money',
                aggregates: ['sum', 'avg', 'min', 'max', 'count', 'countDistinct'],
                currency: 'currency',
            },
            {
                key: 'cost',
                label: 'Cost',
                type: 'money',
                aggregates: ['sum', 'avg', 'min', 'max', 'count', 'countDistinct'],
                currency: 'currency',
            },
            {
                key: 'currency',
                label: 'Currency',
                type: 'text',
                aggregates: ['count', 'countDistinct'],
            },
            {
                key: 'day',
                label: 'Day',
                type: 'date',
                aggregates: ['min', 'max', 'count', 'countDistinct'],
            },
            {
                key: 'status',
                label: 'Status',
                type: 'select',
                aggregates: ['count', 'countDistinct'],
                options: [
                    { value: 'open', label: 'Open' },
                    { value: 'paid', label: 'Paid' },
                ],
            },
        ],
        links: [{ key: 'team', label: 'Team', target: TEAMS_KEY, cardinality: 'toOne' }],
    },
    {
        key: TEAMS_KEY,
        label: 'Stub teams',
        fields: [
            { key: 'id', label: 'ID', type: 'relation', aggregates: ['count', 'countDistinct'] },
            {
                key: 'name',
                label: 'Name',
                type: 'text',
                aggregates: ['min', 'max', 'count', 'countDistinct'],
            },
        ],
        links: [{ key: 'sales', label: 'Sales', target: SALES_KEY, cardinality: 'toMany' }],
    },
])
