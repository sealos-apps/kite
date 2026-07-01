import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useAuth } from '@/contexts/auth-context'
import {
  ColumnFiltersState,
  createColumnHelper,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  PaginationState,
  useReactTable,
  VisibilityState,
} from '@tanstack/react-table'
import {
  Box,
  Copy,
  Database,
  FileArchive,
  Image as ImageIcon,
  Loader2,
  Plus,
  RefreshCw,
  Search,
  Settings2,
  Trash2,
  Upload,
  XCircle,
} from 'lucide-react'
import { Trans, useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'

import {
  ContainerImageUploadResult,
  HelmChart,
  HelmRepository,
  OCIChartUploadResult,
  RepositoryUploadConfig,
} from '@/types/api'
import {
  createHelmRepository,
  deleteHelmRepository,
  fetchRepositoryUploadConfig,
  uploadContainerImageArchive,
  uploadOCIHelmChart,
  useArtifactHubCharts,
  useHelmCharts,
  useHelmRepositories,
} from '@/lib/api'
import { formatDate, translateError } from '@/lib/utils'
import { usePageTitle } from '@/hooks/use-page-title'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'
import { ErrorMessage } from '@/components/error-message'
import { HelmChartIcon } from '@/components/helm-chart-icon'
import { ResourceTableView } from '@/components/resource-table-view'

const allRepositories = 'all'
const artifactHubSource = 'artifacthub'
const repositoriesSource = 'repositories'
const ociSource = 'oci'
const uploadTypeChart = 'chart'
const uploadTypeImage = 'image'
const columnHelper = createColumnHelper<HelmChart>()
type ChartSource =
  | typeof artifactHubSource
  | typeof repositoriesSource
  | typeof ociSource
type UploadType = typeof uploadTypeChart | typeof uploadTypeImage
type HelmChartListSessionState = {
  verifiedPublisherOnly?: boolean
  searchQuery?: string
  repositoryFilter?: string
  pagination?: PaginationState
}

const helmChartListSessionStorageKey = 'kite-helm-chart-list-state'
const defaultPagination: PaginationState = {
  pageIndex: 0,
  pageSize: 20,
}

function readHelmChartListSessionState(): HelmChartListSessionState {
  const value = sessionStorage.getItem(helmChartListSessionStorageKey)
  if (!value) {
    return {}
  }

  try {
    const state = JSON.parse(value) as HelmChartListSessionState
    const pagination = state.pagination

    return {
      verifiedPublisherOnly:
        typeof state.verifiedPublisherOnly === 'boolean'
          ? state.verifiedPublisherOnly
          : undefined,
      searchQuery:
        typeof state.searchQuery === 'string' ? state.searchQuery : undefined,
      repositoryFilter:
        typeof state.repositoryFilter === 'string'
          ? state.repositoryFilter
          : undefined,
      pagination:
        pagination &&
        Number.isInteger(pagination.pageIndex) &&
        pagination.pageIndex >= 0 &&
        Number.isInteger(pagination.pageSize) &&
        pagination.pageSize > 0
          ? pagination
          : undefined,
    }
  } catch {
    return {}
  }
}

function chartDetailPath(chart: HelmChart) {
  const path = `/charts/${encodeURIComponent(chart.repositoryName)}/${encodeURIComponent(chart.name)}`
  if (!chart.source || chart.source === 'repository') {
    return path
  }
  return `${path}?source=${chart.source}`
}

function ChartNameLink({ chart }: { chart: HelmChart }) {
  return (
    <Link
      to={chartDetailPath(chart)}
      className="block truncate font-medium app-link"
    >
      {chart.name}
    </Link>
  )
}

function chartMatchesSearch(chart: HelmChart, query: string) {
  const searchQuery = query.trim().toLowerCase()
  if (!searchQuery) {
    return true
  }

  return [
    chart.name,
    chart.repositoryName,
    chart.version,
    chart.appVersion,
    chart.description,
    ...(chart.keywords || []),
  ].some((value) => value?.toLowerCase().includes(searchQuery))
}

function formatUploadLimit(bytes?: number) {
  if (!bytes || bytes <= 0) {
    return '-'
  }
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let value = bytes
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex += 1
  }
  const digits = value >= 10 || unitIndex === 0 ? 0 : 1
  return `${value.toFixed(digits)} ${units[unitIndex]}`
}

function AddRepositoryDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => Promise<unknown>
}) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [url, setURL] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setIsSubmitting(true)
    try {
      await createHelmRepository({ name, url, username, password })
      toast.success(t('helmCharts.messages.repositoryAdded'))
      setName('')
      setURL('')
      setUsername('')
      setPassword('')
      onOpenChange(false)
      await onCreated()
    } catch (err) {
      setError(translateError(err, t))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{t('helmCharts.actions.addRepository')}</DialogTitle>
            <DialogDescription>
              {t('helmCharts.messages.addRepositoryDescription')}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3">
            <div className="space-y-2">
              <Label htmlFor="helm-repository-name">
                {t('common.fields.name')}
              </Label>
              <Input
                id="helm-repository-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="helm-repository-url">URL</Label>
              <Input
                id="helm-repository-url"
                type="url"
                value={url}
                onChange={(event) => setURL(event.target.value)}
                required
              />
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="helm-repository-username">
                  {t('common.fields.username')}
                </Label>
                <Input
                  id="helm-repository-username"
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="helm-repository-password">
                  {t('common.fields.password')}
                </Label>
                <Input
                  id="helm-repository-password"
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                />
              </div>
            </div>
            {error ? <p className="text-sm text-destructive">{error}</p> : null}
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={isSubmitting}
            >
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {t('sidebar.add')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function UploadRepositoryDialog({
  open,
  onOpenChange,
  onChartUploaded,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onChartUploaded: () => Promise<unknown>
}) {
  const { t } = useTranslation()
  const [uploadType, setUploadType] = useState<UploadType>(uploadTypeChart)
  const [config, setConfig] = useState<RepositoryUploadConfig | null>(null)
  const [isLoadingConfig, setIsLoadingConfig] = useState(false)
  const [chartFile, setChartFile] = useState<File | null>(null)
  const [imageFile, setImageFile] = useState<File | null>(null)
  const [imageRepository, setImageRepository] = useState('')
  const [imageTag, setImageTag] = useState('')
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [lastChartResult, setLastChartResult] =
    useState<OCIChartUploadResult | null>(null)
  const [lastImageResult, setLastImageResult] =
    useState<ContainerImageUploadResult | null>(null)

  useEffect(() => {
    if (!open) {
      return
    }
    setIsLoadingConfig(true)
    setError('')
    void fetchRepositoryUploadConfig()
      .then(setConfig)
      .catch((err) => setError(translateError(err, t)))
      .finally(() => setIsLoadingConfig(false))
  }, [open, t])

  const resetForm = () => {
    setChartFile(null)
    setImageFile(null)
    setImageRepository('')
    setImageTag('')
    setLastChartResult(null)
    setLastImageResult(null)
  }

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen && !isSubmitting) {
      resetForm()
    }
    onOpenChange(nextOpen)
  }

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setIsSubmitting(true)
    setLastChartResult(null)
    setLastImageResult(null)
    try {
      if (uploadType === uploadTypeChart) {
        if (!chartFile) {
          throw new Error(t('helmCharts.messages.selectChartPackage'))
        }
        const result = await uploadOCIHelmChart(chartFile)
        setLastChartResult(result)
        toast.success(
          t('helmCharts.messages.chartUploadSuccess', {
            name: `${result.chartName}:${result.version}`,
          })
        )
        await onChartUploaded()
      } else {
        if (!imageFile) {
          throw new Error(t('helmCharts.messages.selectImageArchive'))
        }
        const result = await uploadContainerImageArchive({
          file: imageFile,
          repository: imageRepository,
          tag: imageTag,
        })
        setLastImageResult(result)
        toast.success(
          t('helmCharts.messages.imageUploadSuccess', {
            name: result.imageRef,
          })
        )
      }
    } catch (err) {
      setError(translateError(err, t))
    } finally {
      setIsSubmitting(false)
    }
  }

  const activeConfig =
    uploadType === uploadTypeChart ? config?.chart : config?.image
  const isConfigured = activeConfig?.configured ?? false

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[620px]">
        <form onSubmit={handleSubmit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{t('helmCharts.actions.upload')}</DialogTitle>
            <DialogDescription>
              {t('helmCharts.messages.uploadDescription')}
            </DialogDescription>
          </DialogHeader>

          <Tabs
            value={uploadType}
            onValueChange={(value) => {
              setUploadType(value as UploadType)
              setError('')
            }}
            className="gap-4"
          >
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value={uploadTypeChart}>
                <FileArchive className="size-4" />
                {t('helmCharts.fields.chartPackage')}
              </TabsTrigger>
              <TabsTrigger value={uploadTypeImage}>
                <ImageIcon className="size-4" />
                {t('helmCharts.fields.containerImage')}
              </TabsTrigger>
            </TabsList>

            <div className="rounded-md border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
              {isLoadingConfig ? (
                <span>{t('common.loading')}</span>
              ) : uploadType === uploadTypeChart ? (
                <span>
                  {t('helmCharts.messages.chartUploadTarget', {
                    target: config?.chart.registryBase || '-',
                    limit: formatUploadLimit(config?.chart.maxBytes),
                  })}
                </span>
              ) : (
                <span>
                  {t('helmCharts.messages.imageUploadTarget', {
                    target: config?.image.registry
                      ? `${config.image.registry}/${config.image.repositoryPrefix || ''}`.replace(
                          /\/$/,
                          ''
                        )
                      : '-',
                    limit: formatUploadLimit(config?.image.maxBytes),
                  })}
                </span>
              )}
            </div>

            <TabsContent value={uploadTypeChart} className="space-y-3">
              <p className="text-sm text-muted-foreground">
                {t('helmCharts.messages.chartUploadDescription')}
              </p>
              <div className="space-y-2">
                <Label htmlFor="helm-chart-upload-file">
                  {t('helmCharts.fields.chartPackage')}
                </Label>
                <Input
                  id="helm-chart-upload-file"
                  type="file"
                  accept=".tgz,application/gzip,application/x-gzip"
                  onChange={(event) =>
                    setChartFile(event.target.files?.[0] ?? null)
                  }
                  disabled={isSubmitting}
                />
              </div>
              {lastChartResult ? (
                <UploadResult value={lastChartResult.chartUrl} />
              ) : null}
            </TabsContent>

            <TabsContent value={uploadTypeImage} className="space-y-3">
              <p className="text-sm text-muted-foreground">
                {t('helmCharts.messages.imageUploadDescription')}
              </p>
              <div className="space-y-2">
                <Label htmlFor="image-upload-file">
                  {t('helmCharts.fields.imageArchive')}
                </Label>
                <Input
                  id="image-upload-file"
                  type="file"
                  accept=".tar,.oci,application/x-tar,application/gzip,application/x-gzip"
                  onChange={(event) =>
                    setImageFile(event.target.files?.[0] ?? null)
                  }
                  disabled={isSubmitting}
                />
              </div>
              <div className="grid gap-3 sm:grid-cols-[1fr_10rem]">
                <div className="space-y-2">
                  <Label htmlFor="image-upload-repository">
                    {t('helmCharts.fields.imageRepository')}
                  </Label>
                  <Input
                    id="image-upload-repository"
                    value={imageRepository}
                    onChange={(event) => setImageRepository(event.target.value)}
                    placeholder={t('helmCharts.placeholders.imageRepository')}
                    disabled={isSubmitting}
                    required={uploadType === uploadTypeImage}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="image-upload-tag">
                    {t('helmCharts.fields.imageTag')}
                  </Label>
                  <Input
                    id="image-upload-tag"
                    value={imageTag}
                    onChange={(event) => setImageTag(event.target.value)}
                    placeholder={t('helmCharts.placeholders.imageTag')}
                    disabled={isSubmitting}
                    required={uploadType === uploadTypeImage}
                  />
                </div>
              </div>
              {lastImageResult ? (
                <UploadResult value={lastImageResult.imageRef} />
              ) : null}
            </TabsContent>
          </Tabs>

          {!isConfigured && !isLoadingConfig ? (
            <p className="text-sm text-destructive">
              {uploadType === uploadTypeChart
                ? t('helmCharts.messages.chartUploadNotConfigured')
                : t('helmCharts.messages.imageUploadNotConfigured')}
            </p>
          ) : null}
          {error ? <p className="text-sm text-destructive">{error}</p> : null}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => handleOpenChange(false)}
              disabled={isSubmitting}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="submit"
              disabled={isSubmitting || isLoadingConfig || !isConfigured}
            >
              {isSubmitting ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Upload className="size-4" />
              )}
              {t('helmCharts.actions.upload')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function UploadResult({ value }: { value: string }) {
  const { t } = useTranslation()
  const copyValue = async () => {
    await navigator.clipboard.writeText(value)
    toast.success(t('common.copied'))
  }

  return (
    <div className="flex items-center gap-2 rounded-md border bg-muted/20 p-2">
      <code className="min-w-0 flex-1 truncate text-xs">{value}</code>
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="size-8 shrink-0"
        onClick={() => void copyValue()}
        aria-label={t('helmCharts.actions.copyReference')}
      >
        <Copy className="size-4" />
      </Button>
    </div>
  )
}

export function HelmChartListPage() {
  const { t } = useTranslation()
  const { user, helmArtifactHubEnabled } = useAuth()
  const [initialSessionState] = useState(readHelmChartListSessionState)
  const [chartSource, setChartSource] = useState<ChartSource>(ociSource)
  const [verifiedPublisherOnly, setVerifiedPublisherOnly] = useState(
    initialSessionState.verifiedPublisherOnly ?? false
  )
  const [searchQuery, setSearchQuery] = useState(
    initialSessionState.searchQuery ?? ''
  )
  const [repositoryFilter, setRepositoryFilter] = useState(
    initialSessionState.repositoryFilter ?? allRepositories
  )
  const [dialogOpen, setDialogOpen] = useState(false)
  const [uploadDialogOpen, setUploadDialogOpen] = useState(false)
  const [repositoryToDelete, setRepositoryToDelete] =
    useState<HelmRepository | null>(null)
  const [isDeletingRepository, setIsDeletingRepository] = useState(false)
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
  const [pagination, setPagination] = useState<PaginationState>(
    initialSessionState.pagination ?? defaultPagination
  )
  const selectedRepository =
    repositoryFilter === allRepositories ? undefined : repositoryFilter
  const isArtifactHubSource = chartSource === artifactHubSource
  const isOCISource = chartSource === ociSource
  const canManageRepositories = user?.isAdmin() ?? false

  usePageTitle(t('nav.helmCharts'))

  useEffect(() => {
    if (!helmArtifactHubEnabled && chartSource === artifactHubSource) {
      setChartSource(ociSource)
    }
  }, [chartSource, helmArtifactHubEnabled])

  useEffect(() => {
    sessionStorage.setItem(
      helmChartListSessionStorageKey,
      JSON.stringify({
        verifiedPublisherOnly,
        searchQuery,
        repositoryFilter,
        pagination,
      })
    )
  }, [verifiedPublisherOnly, searchQuery, repositoryFilter, pagination])

  const { data: repositories = [], refetch: refetchRepositories } =
    useHelmRepositories()
  const selectedRepositoryItem = repositories.find(
    (repository) => repository.name === selectedRepository
  )
  const localChartsQuery = useHelmCharts({
    repository: isOCISource ? undefined : selectedRepository,
    query: isOCISource ? searchQuery : undefined,
    source: isOCISource ? 'oci' : 'repository',
    enabled: !isArtifactHubSource,
  })
  const artifactHubChartsQuery = useArtifactHubCharts({
    query: searchQuery,
    verifiedPublisher: verifiedPublisherOnly,
    limit: pagination.pageSize,
    offset: pagination.pageIndex * pagination.pageSize,
    enabled: helmArtifactHubEnabled && isArtifactHubSource,
  })
  const activeChartsQuery = isArtifactHubSource
    ? artifactHubChartsQuery
    : localChartsQuery
  const {
    data,
    isLoading,
    isFetching,
    isError,
    error,
    refetch: refetchCharts,
  } = activeChartsQuery
  const charts = data?.items || []
  const totalRowCount = isArtifactHubSource
    ? (data?.total ?? charts.length)
    : charts.length

  const columns = useMemo(
    () => [
      columnHelper.accessor('name', {
        header: t('helm.fields.chart'),
        enableHiding: false,
        cell: ({ row }) => (
          <div className="flex min-w-[22rem] items-center gap-3">
            <HelmChartIcon
              icon={row.original.icon}
              name={row.original.name}
              className="size-8"
            />
            <div className="min-w-0">
              <ChartNameLink chart={row.original} />
              <div className="truncate text-xs text-muted-foreground">
                {row.original.repositoryName}
              </div>
            </div>
          </div>
        ),
      }),
      columnHelper.accessor('version', {
        header: t('helm.fields.version'),
        cell: ({ getValue }) => (
          <span className="tabular-nums">{getValue() || '-'}</span>
        ),
      }),
      columnHelper.accessor('appVersion', {
        header: t('helm.fields.appVersion'),
        cell: ({ getValue }) => (
          <span className="tabular-nums">{getValue() || '-'}</span>
        ),
      }),
      columnHelper.accessor('description', {
        header: t('common.fields.description'),
        cell: ({ getValue }) => (
          <span className="block whitespace-normal break-words text-left text-sm leading-5 text-muted-foreground line-clamp-2">
            {getValue() || '-'}
          </span>
        ),
      }),
      columnHelper.accessor('updatedAt', {
        header: t('helmCharts.fields.updatedAt'),
        cell: ({ getValue }) => (
          <span className="text-sm text-muted-foreground tabular-nums">
            {getValue() ? formatDate(getValue() || '') : '-'}
          </span>
        ),
      }),
    ],
    [t]
  )

  const table = useReactTable({
    data: charts,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    onColumnFiltersChange: setColumnFilters,
    onColumnVisibilityChange: setColumnVisibility,
    onPaginationChange: setPagination,
    enableSorting: false,
    manualPagination: isArtifactHubSource,
    pageCount: isArtifactHubSource
      ? Math.ceil(totalRowCount / pagination.pageSize) || 0
      : undefined,
    getRowId: (chart) =>
      `${chart.source || repositoriesSource}/${chart.repositoryUrl}/${chart.name}`,
    globalFilterFn: (row, _columnId, value) =>
      chartMatchesSearch(row.original, String(value)),
    state: {
      columnFilters,
      globalFilter: isArtifactHubSource ? '' : searchQuery,
      columnVisibility,
      pagination,
    },
    autoResetPageIndex: false,
  })

  const handleCreated = async () => {
    await Promise.all([refetchRepositories(), localChartsQuery.refetch()])
  }

  const handleChartUploaded = async () => {
    setChartSource(ociSource)
    await localChartsQuery.refetch()
  }

  const handleDeleteRepository = async () => {
    if (!repositoryToDelete) {
      return
    }
    setIsDeletingRepository(true)
    try {
      await deleteHelmRepository(repositoryToDelete.id)
      toast.success(t('helmCharts.messages.repositoryDeleted'))
      setRepositoryFilter(allRepositories)
      setRepositoryToDelete(null)
      await Promise.all([refetchRepositories(), localChartsQuery.refetch()])
    } catch (err) {
      toast.error(translateError(err, t))
    } finally {
      setIsDeletingRepository(false)
    }
  }

  const updateChartSource = (value: string) => {
    if (!value) {
      return
    }
    if (value === artifactHubSource && !helmArtifactHubEnabled) {
      return
    }
    setChartSource(value as ChartSource)
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }

  const updateSearchQuery = (value: string) => {
    setSearchQuery(value)
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }

  const updateVerifiedPublisherOnly = (value: boolean) => {
    setVerifiedPublisherOnly(value)
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }

  const updateRepositoryFilter = (value: string) => {
    setRepositoryFilter(value)
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }

  const filteredRowCount = isArtifactHubSource
    ? charts.length
    : table.getFilteredRowModel().rows.length
  const emptyState = (() => {
    if (isLoading && charts.length === 0) {
      return (
        <div className="flex h-72 flex-col items-center justify-center">
          <div className="mb-4 rounded-full bg-muted/30 p-6">
            <Database className="size-12 text-muted-foreground" />
          </div>
          <h3 className="mb-1 text-lg font-medium">
            {t('common.messages.loading')}
          </h3>
        </div>
      )
    }

    if (isError) {
      return (
        <ErrorMessage
          resourceName={t('nav.helmCharts')}
          error={error}
          refetch={refetchCharts}
        />
      )
    }

    if (charts.length === 0) {
      return (
        <div className="flex h-72 flex-col items-center justify-center">
          <div className="mb-4 rounded-full bg-muted/30 p-6">
            <Box className="size-12 text-muted-foreground" />
          </div>
          <h3 className="mb-1 text-lg font-medium">
            {t('helmCharts.messages.noCharts')}
          </h3>
          {isOCISource && canManageRepositories ? (
            <Button
              variant="outline"
              className="mt-4"
              onClick={() => setUploadDialogOpen(true)}
            >
              <Upload className="size-4" />
              {t('helmCharts.actions.upload')}
            </Button>
          ) : null}
          {!isArtifactHubSource && !isOCISource && canManageRepositories ? (
            <Button
              variant="outline"
              className="mt-4"
              onClick={() => setDialogOpen(true)}
            >
              <Plus className="size-4" />
              {t('helmCharts.actions.addRepository')}
            </Button>
          ) : null}
        </div>
      )
    }

    return null
  })()

  return (
    <>
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
            <ToggleGroup
              type="single"
              value={chartSource}
              onValueChange={updateChartSource}
              aria-label={t('common.fields.source')}
              className="h-9 shrink-0 gap-0 overflow-hidden rounded-md border bg-muted/30 p-0.5 shadow-xs"
            >
              <ToggleGroupItem
                value={ociSource}
                className="h-8 min-w-[5.5rem] flex-none rounded-sm border-0 px-3 text-muted-foreground shadow-none hover:bg-background/70 hover:text-foreground data-[state=on]:bg-background data-[state=on]:text-foreground data-[state=on]:shadow-xs"
              >
                {t('helmCharts.filters.oci')}
              </ToggleGroupItem>
              <ToggleGroupItem
                value={repositoriesSource}
                className="h-8 min-w-[4.25rem] flex-none rounded-sm border-0 px-3 text-muted-foreground shadow-none hover:bg-background/70 hover:text-foreground data-[state=on]:bg-background data-[state=on]:text-foreground data-[state=on]:shadow-xs"
              >
                {t('helmCharts.filters.repositories')}
              </ToggleGroupItem>
              {helmArtifactHubEnabled ? (
                <ToggleGroupItem
                  value={artifactHubSource}
                  className="h-8 min-w-[7.75rem] flex-none rounded-sm border-0 px-3 text-muted-foreground shadow-none hover:bg-background/70 hover:text-foreground data-[state=on]:bg-background data-[state=on]:text-foreground data-[state=on]:shadow-xs"
                >
                  {t('helmCharts.filters.artifactHub')}
                </ToggleGroupItem>
              ) : null}
            </ToggleGroup>
            {!isArtifactHubSource && !isOCISource ? (
              <div className="flex w-full items-center gap-2 sm:w-auto">
                <Select
                  value={repositoryFilter}
                  onValueChange={updateRepositoryFilter}
                >
                  <SelectTrigger className="w-full sm:w-[220px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={allRepositories}>
                      {t('helmCharts.filters.allRepositories')}
                    </SelectItem>
                    {repositories.map((repository) => (
                      <SelectItem key={repository.id} value={repository.name}>
                        {repository.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {selectedRepositoryItem && canManageRepositories ? (
                  <Button
                    variant="outline"
                    size="icon"
                    aria-label={t('helmCharts.actions.deleteRepository')}
                    onClick={() =>
                      setRepositoryToDelete(selectedRepositoryItem)
                    }
                  >
                    <Trash2 className="size-4" />
                  </Button>
                ) : null}
              </div>
            ) : null}
            {isArtifactHubSource ? (
              <div className="flex h-9 items-center gap-2 rounded-md border px-3">
                <Switch
                  id="artifacthub-verified-publisher"
                  checked={verifiedPublisherOnly}
                  onCheckedChange={updateVerifiedPublisherOnly}
                />
                <Label
                  htmlFor="artifacthub-verified-publisher"
                  className="whitespace-nowrap text-sm font-normal"
                >
                  {t('helmCharts.filters.verifiedPublisher')}
                </Label>
              </div>
            ) : null}
            <Button
              variant="outline"
              size="icon"
              disabled={isFetching}
              aria-label={t('common.refresh')}
              onClick={() => void refetchCharts()}
            >
              <RefreshCw className="size-4" />
            </Button>
          </div>

          <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row sm:items-center sm:justify-end">
            <div className="flex w-full items-center gap-2 sm:w-auto">
              <div className="relative min-w-0 flex-1 sm:w-[280px]">
                <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder={t('helmCharts.placeholders.search')}
                  value={searchQuery}
                  onChange={(event) => updateSearchQuery(event.target.value)}
                  className="w-full pl-9 pr-4"
                />
              </div>
              {searchQuery ? (
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => updateSearchQuery('')}
                  className="size-9"
                  aria-label={t('common.close')}
                >
                  <XCircle className="size-4" />
                </Button>
              ) : null}
            </div>

            <div className="flex flex-wrap items-center gap-2 sm:justify-end">
              {canManageRepositories ? (
                <Button
                  variant={isOCISource ? 'default' : 'outline'}
                  onClick={() => setUploadDialogOpen(true)}
                >
                  <Upload className="size-4" />
                  {t('helmCharts.actions.upload')}
                </Button>
              ) : null}
              {!isArtifactHubSource && !isOCISource && canManageRepositories ? (
                <Button onClick={() => setDialogOpen(true)}>
                  <Plus className="size-4" />
                  {t('helmCharts.actions.addRepository')}
                </Button>
              ) : null}

              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="outline"
                    size="icon"
                    aria-label={t('resourceTable.toggleColumns')}
                  >
                    <Settings2 className="size-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuLabel>
                    {t('resourceTable.toggleColumns')}
                  </DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  {table
                    .getAllLeafColumns()
                    .filter((column) => column.getCanHide())
                    .map((column) => {
                      const header = column.columnDef.header
                      const headerText =
                        typeof header === 'string' ? header : column.id

                      return (
                        <DropdownMenuCheckboxItem
                          key={column.id}
                          className="capitalize"
                          checked={column.getIsVisible()}
                          onCheckedChange={(value) =>
                            column.toggleVisibility(!!value)
                          }
                        >
                          {headerText}
                        </DropdownMenuCheckboxItem>
                      )
                    })}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        </div>

        {isArtifactHubSource && helmArtifactHubEnabled ? (
          <p className="text-pretty text-xs text-muted-foreground">
            <Trans
              i18nKey="helmCharts.messages.artifactHubTrafficNotice"
              components={{
                artifactHub: (
                  <a
                    href="https://artifacthub.io"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="app-link"
                  />
                ),
              }}
            />
          </p>
        ) : null}

        <ResourceTableView
          table={table}
          columnCount={columns.length}
          isLoading={isLoading}
          data={charts}
          fitViewportHeight={true}
          emptyState={emptyState}
          hasActiveFilters={Boolean(searchQuery)}
          filteredRowCount={filteredRowCount}
          totalRowCount={totalRowCount}
          searchQuery={searchQuery}
          pagination={pagination}
          setPagination={setPagination}
          shrinkFirstColumn={false}
          showAllPageSize={!isArtifactHubSource}
        />
      </div>

      <AddRepositoryDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onCreated={handleCreated}
      />
      <UploadRepositoryDialog
        open={uploadDialogOpen}
        onOpenChange={setUploadDialogOpen}
        onChartUploaded={handleChartUploaded}
      />
      <DeleteConfirmationDialog
        open={Boolean(repositoryToDelete)}
        onOpenChange={(open) => {
          if (!open) {
            setRepositoryToDelete(null)
          }
        }}
        resourceName={repositoryToDelete?.name || ''}
        resourceType={t('helmCharts.fields.repository')}
        onConfirm={() => void handleDeleteRepository()}
        isDeleting={isDeletingRepository}
        additionalNote={t('helmCharts.messages.deleteRepositoryDescription')}
      />
    </>
  )
}
