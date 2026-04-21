import { useCallback, useMemo } from 'react'
import { createColumnHelper } from '@tanstack/react-table'
import { HorizontalPodAutoscaler } from 'kubernetes-types/autoscaling/v2'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'

import { formatDate } from '@/lib/utils'
import { ResourceTable } from '@/components/resource-table'

function getHpaTargetInfo(hpa: HorizontalPodAutoscaler): string {
  if (!hpa.spec?.scaleTargetRef) {
    return '-'
  }
  const { kind, name } = hpa.spec.scaleTargetRef
  return `${kind}/${name}`
}

function getCurrentReplicas(hpa: HorizontalPodAutoscaler): number {
  return hpa.status?.currentReplicas || 0
}

function getMetricUtilization(hpa: HorizontalPodAutoscaler): string {
  if (!hpa.status?.currentMetrics || hpa.status.currentMetrics.length === 0) {
    return '-'
  }

  const metrics = hpa.status.currentMetrics
  const results: string[] = []

  metrics.forEach((metric) => {
    if ('resource' in metric && metric.resource) {
      const current = metric.resource.current?.averageUtilization || 0
      const target =
        hpa.spec?.metrics?.find(
          (m) => 'resource' in m && m.resource?.name === metric.resource?.name
        )?.resource?.target?.averageUtilization || 0
      results.push(`${metric.resource.name}: ${current}% / ${target}%`)
    } else if (metric.type === 'Pods') {
      results.push('Pods metric')
    } else if (metric.type === 'Object') {
      results.push('Object metric')
    } else if (metric.type === 'External') {
      results.push('External metric')
    }
  })

  return results.join(', ')
}

export function HorizontalPodAutoscalerListPage() {
  const { t } = useTranslation()
  const columnHelper = createColumnHelper<HorizontalPodAutoscaler>()

  const columns = useMemo(
    () => [
      columnHelper.accessor('metadata.name', {
        header: t('common.name'),
        cell: ({ row }) => (
          <div className="font-medium text-blue-500 hover:underline">
            <Link
              to={`/horizontalpodautoscalers/${row.original.metadata!.namespace}/${
                row.original.metadata!.name
              }`}
            >
              {row.original.metadata!.name}
            </Link>
          </div>
        ),
      }),
      columnHelper.accessor((row) => getHpaTargetInfo(row), {
        header: t('hpa.target'),
        cell: ({ getValue }) => getValue(),
      }),
      columnHelper.accessor((row) => row.spec?.minReplicas, {
        id: 'minReplicas',
        header: t('hpa.minPods'),
        cell: ({ getValue }) => getValue() || '-',
      }),
      columnHelper.accessor((row) => row.spec?.maxReplicas, {
        id: 'maxReplicas',
        header: t('hpa.maxPods'),
        cell: ({ getValue }) => getValue() || '-',
      }),
      columnHelper.accessor((row) => getCurrentReplicas(row), {
        id: 'currentReplicas',
        header: t('hpa.currentPods'),
        cell: ({ getValue }) => getValue(),
      }),
      columnHelper.accessor((row) => getMetricUtilization(row), {
        id: 'metrics',
        header: t('hpa.metrics'),
        cell: ({ getValue }) => (
          <span className="text-muted-foreground text-sm">{getValue()}</span>
        ),
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

  const horizontalPodAutoscalerSearchFilter = useCallback(
    (hpa: HorizontalPodAutoscaler, query: string) => {
      const queryLower = query.toLowerCase()
      return (
        hpa.metadata!.name!.toLowerCase().includes(queryLower) ||
        (hpa.metadata!.namespace?.toLowerCase() || '').includes(queryLower) ||
        getHpaTargetInfo(hpa).toLowerCase().includes(queryLower)
      )
    },
    []
  )

  return (
    <ResourceTable
      resourceName={t('nav.horizontalpodautoscalers')}
      resourceType="horizontalpodautoscalers"
      columns={columns}
      searchQueryFilter={horizontalPodAutoscalerSearchFilter}
    />
  )
}
