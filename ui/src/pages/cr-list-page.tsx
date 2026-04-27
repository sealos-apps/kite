import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { createColumnHelper } from '@tanstack/react-table'
import * as yaml from 'js-yaml'
import { CustomResourceDefinition } from 'kubernetes-types/apiextensions/v1'
import { get } from 'lodash'
import { Eye } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, useParams } from 'react-router-dom'
import { toast } from 'sonner'

import { CustomResource, ResourceType } from '@/types/api'
import {
  BuiltinSidebarCRD,
  fetchResource,
  useBuiltinSidebarCRDs,
} from '@/lib/api'
import { formatDate } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ResourceTable } from '@/components/resource-table'
import { YamlEditor } from '@/components/yaml-editor'

function buildCRDFromBuiltinSidebarInfo(
  crd: BuiltinSidebarCRD
): CustomResourceDefinition {
  return {
    apiVersion: 'apiextensions.k8s.io/v1',
    kind: 'CustomResourceDefinition',
    metadata: {
      name: crd.name,
    },
    spec: {
      group: crd.group,
      names: {
        kind: crd.kind,
        plural: crd.name.split('.')[0],
      },
      scope: crd.scope as CustomResourceDefinition['spec']['scope'],
      versions: crd.versions.map((version) => ({
        name: version.name,
        served: version.served,
        storage: version.storage,
        schema: {
          openAPIV3Schema: {
            type: 'object',
          },
        },
        additionalPrinterColumns: version.additionalPrinterColumns,
      })),
    },
  } as CustomResourceDefinition
}

export function CRListPage() {
  const { t } = useTranslation()
  const [isYamlDialogOpen, setIsYamlDialogOpen] = useState(false)
  const [yamlContent, setYamlContent] = useState('')
  const { crd } = useParams<{ crd: string }>()
  const { currentClusterInfo } = useCluster()
  const isNamespaceScopedCluster = !!currentClusterInfo?.namespaceScoped
  const {
    data: builtinCRDs,
    isLoading: isLoadingBuiltinCRDs,
    isFetched: hasFetchedBuiltinCRDs,
  } = useBuiltinSidebarCRDs({ disable: !crd })
  const builtinCRDData = useMemo<CustomResourceDefinition | undefined>(() => {
    const builtinCRD = builtinCRDs?.find((item) => item.name === crd)
    if (!builtinCRD) {
      return undefined
    }
    return buildCRDFromBuiltinSidebarInfo(builtinCRD)
  }, [builtinCRDs, crd])
  const { data: fullCRDData, isLoading: isLoadingFullCRD } = useQuery({
    queryKey: ['crds', crd],
    queryFn: () =>
      fetchResource<CustomResourceDefinition>('crds', crd!, '_all'),
    enabled: !!crd && !isNamespaceScopedCluster,
  })
  const crdData = fullCRDData ?? builtinCRDData
  const isClusterScopeBlocked =
    isNamespaceScopedCluster &&
    hasFetchedBuiltinCRDs &&
    (!builtinCRDData || builtinCRDData.spec.scope === 'Cluster')
  const isLoadingCRD = isNamespaceScopedCluster
    ? isLoadingBuiltinCRDs
    : isLoadingFullCRD

  useEffect(() => {
    if (!isClusterScopeBlocked) return
    toast.warning(
      'This cluster is namespace-scoped. Cluster-level resources are disabled.',
      {
        id: 'cluster-scope-resource-guard',
      }
    )
  }, [isClusterScopeBlocked])

  const columnHelper = createColumnHelper<CustomResource>()
  const handleViewYaml = useCallback((crd: CustomResourceDefinition) => {
    setYamlContent(yaml.dump(crd, { indent: 2 }))
    setIsYamlDialogOpen(true)
  }, [])
  const extraToolbars = useMemo(() => {
    if (!fullCRDData) {
      return []
    }

    return [
      <Button
        variant="outline"
        size="default"
        onClick={() => {
          handleViewYaml(fullCRDData)
        }}
      >
        <Eye className="h-4 w-4 mr-1" />
        {t('common.yaml')}
      </Button>,
    ]
  }, [fullCRDData, handleViewYaml, t])
  const columns = useMemo(() => {
    const baseColumns = [
      columnHelper.accessor('metadata.name', {
        header: t('common.name'),
        cell: ({ row }) => {
          const resource = row.original
          const namespace = resource.metadata?.namespace
          const path = namespace
            ? `/crds/${crd}/${namespace}/${resource.metadata.name}`
            : `/crds/${crd}/${resource.metadata.name}`

          return (
            <div className="font-medium text-blue-500 hover:underline">
              <Link to={path}>{resource.metadata.name}</Link>
            </div>
          )
        },
      }),
    ]
    const additionalColumns =
      crdData?.spec.versions[0].additionalPrinterColumns?.map(
        (printerColumn) => {
          const jsonPath = printerColumn.jsonPath.startsWith('.')
            ? printerColumn.jsonPath.slice(1)
            : printerColumn.jsonPath

          return columnHelper.accessor((row) => get(row, jsonPath), {
            id: jsonPath || printerColumn.name,
            header: printerColumn.name,
            cell: ({ getValue }) => {
              const type = printerColumn.type
              const value = getValue()
              if (!value) {
                return <span className="text-sm text-muted-foreground">-</span>
              }
              if (type === 'date') {
                return (
                  <span className="text-sm text-muted-foreground">
                    {formatDate(value)}
                  </span>
                )
              }
              return (
                <span className="text-sm text-muted-foreground">{value}</span>
              )
            },
          })
        }
      )
    return [...baseColumns, ...(additionalColumns ?? [])]
  }, [columnHelper, crd, crdData?.spec.versions, t])

  const searchQueryFilter = useCallback((cr: CustomResource, query: string) => {
    const searchFields = [
      cr.metadata?.name || '',
      cr.metadata?.namespace || '',
      cr.kind || '',
      cr.apiVersion || '',
      ...(cr.metadata?.labels ? Object.keys(cr.metadata.labels) : []),
      ...(cr.metadata?.labels ? Object.values(cr.metadata.labels) : []),
    ]

    return searchFields.some((field) =>
      field.toLowerCase().includes(query.toLowerCase())
    )
  }, [])

  if (isClusterScopeBlocked) {
    return <Navigate to="/" replace />
  }

  if (isLoadingCRD) {
    return <div>{t('common.loading')}</div>
  }

  if (!crdData) {
    return <div>Error: CRD name is required</div>
  }

  return (
    <>
      <ResourceTable
        resourceName={crdData.spec.names.kind || 'Custom Resources'}
        resourceType={crd as ResourceType}
        columns={columns}
        clusterScope={crdData.spec.scope === 'Cluster'}
        searchQueryFilter={searchQueryFilter}
        extraToolbars={extraToolbars}
      />

      <Dialog open={isYamlDialogOpen} onOpenChange={setIsYamlDialogOpen}>
        <DialogContent className="sm:max-w-4xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              YAML Configuration: {crdData?.metadata?.name ?? 'Unknown'}
            </DialogTitle>
          </DialogHeader>
          <YamlEditor
            value={yamlContent}
            readOnly={true}
            showControls={false}
            minHeight={600}
          />
        </DialogContent>
      </Dialog>
    </>
  )
}
