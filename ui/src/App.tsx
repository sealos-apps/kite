import './App.css'

import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { IconRefresh, IconSettings } from '@tabler/icons-react'
import { Outlet, useLocation, useNavigate, useSearchParams } from 'react-router-dom'

import { AppSidebar } from './components/app-sidebar'
import { GlobalSearch } from './components/global-search'
import {
  GlobalSearchProvider,
  useGlobalSearch,
} from './components/global-search-provider'
import { SiteHeader } from './components/site-header'
import { Alert, AlertDescription, AlertTitle } from './components/ui/alert'
import { Button } from './components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from './components/ui/card'
import { SidebarInset, SidebarProvider } from './components/ui/sidebar'
import { Toaster } from './components/ui/sonner'
import { ClusterProvider } from './contexts/cluster-context'
import { useCluster } from './hooks/use-cluster'
import { apiClient } from './lib/api-client'

function ClusterAwareApp() {
  const { t } = useTranslation()
  const { currentCluster, isLoading, error, hasReachableCluster, refetchClusters } =
    useCluster()
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => {
    apiClient.setClusterProvider(() => {
      return currentCluster || localStorage.getItem('current-cluster')
    })
  }, [currentCluster])

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="flex items-center space-x-2">
          <div className="h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600" />
          <span>{t('cluster.loading')}</span>
        </div>
      </div>
    )
  }

  if ((error || !hasReachableCluster) && location.pathname !== '/settings') {
    const hasFetchError = Boolean(error)
    return (
      <div className="min-h-screen bg-background p-4 lg:p-6">
        <div className="mx-auto flex w-full max-w-2xl flex-col gap-4 pt-8">
          <Card className="border-border bg-card">
            <CardHeader className="space-y-2">
              <CardTitle className="text-xl">
                {hasFetchError
                  ? t('cluster.unavailable.titleFetchError')
                  : t('cluster.unavailable.titleNoReachable')}
              </CardTitle>
              <p className="text-sm text-muted-foreground">
                {hasFetchError
                  ? t('cluster.unavailable.descriptionFetchError')
                  : t('cluster.unavailable.descriptionNoReachable')}
              </p>
            </CardHeader>
            <CardContent className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                variant="default"
                onClick={() => navigate('/settings?tab=clusters')}
              >
                <IconSettings className="h-4 w-4" />
                {t('cluster.unavailable.actions.goToClusters')}
              </Button>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  void refetchClusters()
                }}
              >
                <IconRefresh className="h-4 w-4" />
                {t('cluster.unavailable.actions.retry')}
              </Button>
            </CardContent>
          </Card>
          {error ? (
            <Alert variant="destructive">
              <AlertTitle>{t('common.error')}</AlertTitle>
              <AlertDescription>
                {t('cluster.error', { error: error.message })}
              </AlertDescription>
            </Alert>
          ) : null}
        </div>
      </div>
    )
  }

  return <AppContent />
}

function AppContent() {
  const { isOpen, closeSearch } = useGlobalSearch()
  const [searchParams] = useSearchParams()
  const isIframe = searchParams.get('iframe') === 'true'

  if (isIframe) {
    return <Outlet />
  }

  return (
    <>
      <SidebarProvider>
        <AppSidebar variant="inset" />
        <SidebarInset className="h-screen overflow-y-auto scrollbar-hide">
          <SiteHeader />
          <div className="@container/main">
            <div className="flex flex-col gap-4 py-4 md:gap-6">
              <div className="px-4 lg:px-6">
                <Outlet />
              </div>
            </div>
          </div>
        </SidebarInset>
      </SidebarProvider>
      <GlobalSearch open={isOpen} onOpenChange={closeSearch} />
      <Toaster />
    </>
  )
}

function App() {
  return (
    <ClusterProvider>
      <GlobalSearchProvider>
        <ClusterAwareApp />
      </GlobalSearchProvider>
    </ClusterProvider>
  )
}

export default App
