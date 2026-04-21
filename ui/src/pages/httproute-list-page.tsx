import { useCallback, useMemo } from 'react'
import { createColumnHelper } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import { ResourceType } from '@/types/api'
import { HTTPRoute } from '@/types/gateway'
import { formatDate } from '@/lib/utils'
import { ResourceTable } from '@/components/resource-table'

export function HTTPRouteListPage() {
  const { t } = useTranslation()
  // Define column helper outside of any hooks
  const columnHelper = createColumnHelper<HTTPRoute>()

  const columns = useMemo(
    () => [
      columnHelper.accessor('metadata.name', {
        header: t('common.name'),
        cell: ({ row }) => (
          <div className="font-medium text-blue-500 hover:underline">
            <Link
              to={`/httproutes/${row.original.metadata!.namespace}/${row.original.metadata!.name}`}
            >
              {row.original.metadata!.name}
            </Link>
          </div>
        ),
      }),
      columnHelper.accessor('spec.hostnames', {
        header: t('httproute.hostnames'),
        cell: ({ row }) => row.original.spec?.hostnames?.join(', ') || '-',
      }),
      columnHelper.accessor('metadata.creationTimestamp', {
        header: t('common.created'),
        cell: ({ getValue }) => {
          const dateStr = formatDate(getValue() || '')

          return (
            <span className="text-muted-foreground text-sm">{dateStr}</span>
          )
        },
      }),
    ],
    [columnHelper, t]
  )

  const filter = useCallback((ns: HTTPRoute, query: string) => {
    return ns.metadata!.name!.toLowerCase().includes(query)
  }, [])

  return (
    <ResourceTable
      resourceName={t('nav.httproutes')}
      resourceType={'httproutes' as ResourceType}
      columns={columns}
      searchQueryFilter={filter}
    />
  )
}
