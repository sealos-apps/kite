import { useEffect } from 'react'
import { Navigate, useLocation, useParams } from 'react-router-dom'
import { toast } from 'sonner'

import { clusterScopeResources, ResourceType } from '@/types/api'
import { useCluster } from '@/hooks/use-cluster'
import { usePageTitle } from '@/hooks/use-page-title'
import { Card, CardContent } from '@/components/ui/card'

import { CronJobDetail } from './cronjob-detail'
import { DaemonSetDetail } from './daemonset-detail'
import { DeploymentDetail } from './deployment-detail'
import { JobDetail } from './job-detail'
import { NodeDetail } from './node-detail'
import { PodDetail } from './pod-detail'
import { SecretDetail } from './secret-detail'
import { ServiceDetail } from './service-detail'
import { SimpleResourceDetail } from './simple-resource-detail'
import { StatefulSetDetail } from './statefulset-detail'

function getResourceTypeName(resource: string): string {
  const resourceMap: Record<string, string> = {
    deployments: 'Deploy',
    daemonsets: 'Daemon',
    statefulsets: 'STS',
    jobs: 'Job',
    persistentvolumeclaims: 'PVC',
    persistentvolumes: 'PV',
    horizontalpodautoscalers: 'HPA',
  }
  return (
    resourceMap[resource] ||
    resource.replace(/s$/, '').charAt(0).toUpperCase() + resource.slice(1, -1)
  )
}

export function ResourceDetail() {
  const { resource, namespace, name } = useParams()
  const location = useLocation()
  const {
    clusters,
    currentCluster,
    currentClusterInfo,
    isLoading: isClusterLoading,
  } = useCluster()
  const isClusterContextPending =
    isClusterLoading ||
    (clusters.length > 0 && (!currentCluster || !currentClusterInfo))
  const isClusterScopedResource =
    !!resource && clusterScopeResources.includes(resource as ResourceType)
  const isClusterScopeBlocked =
    !!currentClusterInfo?.namespaceScoped && isClusterScopedResource
  const scopedNamespace =
    currentClusterInfo?.namespaceScoped && currentClusterInfo.namespace
      ? currentClusterInfo.namespace
      : undefined
  const shouldUseScopedNamespace =
    !!scopedNamespace && !!resource && !isClusterScopedResource
  const isOutsideScopedNamespace =
    shouldUseScopedNamespace && !!namespace && namespace !== scopedNamespace
  const effectiveNamespace = shouldUseScopedNamespace
    ? scopedNamespace
    : namespace

  useEffect(() => {
    if (!isClusterScopeBlocked) return
    toast.warning(
      'This cluster is namespace-scoped. Cluster-level resources are disabled.',
      {
        id: 'cluster-scope-resource-guard',
      }
    )
  }, [isClusterScopeBlocked])

  useEffect(() => {
    if (!isOutsideScopedNamespace) return
    toast.warning(
      'Workspace namespace changed. Showing resources from the current workspace.',
      {
        id: 'namespace-scope-route-guard',
      }
    )
  }, [isOutsideScopedNamespace])

  const resourceTypeName = resource ? getResourceTypeName(resource) : ''
  const pageTitle =
    resource && name ? `${name} (${resourceTypeName})` : 'Resource'
  usePageTitle(pageTitle)

  if (isClusterContextPending) {
    return (
      <div className="p-6">
        <Card>
          <CardContent className="pt-6">
            <div className="text-center text-muted-foreground">Loading...</div>
          </CardContent>
        </Card>
      </div>
    )
  }

  if (isClusterScopeBlocked) {
    return <Navigate to="/" replace />
  }

  if (isOutsideScopedNamespace) {
    const isCustomResourceRoute = location.pathname
      .split('/')
      .filter(Boolean)[0] === 'crds'
    const listPath = isCustomResourceRoute ? `/crds/${resource}` : `/${resource}`
    return <Navigate to={listPath} replace />
  }

  if (!resource || !name) {
    return (
      <div className="p-6">
        <Card>
          <CardContent className="pt-6">
            <div className="text-center text-muted-foreground">
              Invalid parameters. name are required.
            </div>
          </CardContent>
        </Card>
      </div>
    )
  }

  switch (resource) {
    case 'deployments':
      return <DeploymentDetail namespace={effectiveNamespace!} name={name} />
    case 'pods':
      return <PodDetail namespace={effectiveNamespace!} name={name} />
    case 'daemonsets':
      return <DaemonSetDetail namespace={effectiveNamespace!} name={name} />
    case 'statefulsets':
      return <StatefulSetDetail namespace={effectiveNamespace!} name={name} />
    case 'jobs':
      return <JobDetail namespace={effectiveNamespace!} name={name} />
    case 'cronjobs':
      return <CronJobDetail namespace={effectiveNamespace!} name={name} />
    case 'secrets':
      return <SecretDetail namespace={effectiveNamespace!} name={name} />
    case 'nodes':
      return <NodeDetail name={name} />
    case 'services':
      return <ServiceDetail namespace={effectiveNamespace!} name={name} />
    default:
      return (
        <SimpleResourceDetail
          resourceType={resource as ResourceType}
          namespace={effectiveNamespace}
          name={name}
        />
      )
  }
}
