import { useEffect, useMemo, useState } from 'react'
import { IconEdit, IconServer, IconTrash, IconUpload } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as yaml from 'js-yaml'

import { Cluster } from '@/types/api'
import { ClusterCreateRequest } from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'

interface ClusterDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  cluster?: Cluster | null
  onSubmit: (clusterData: ClusterCreateRequest | ClusterCreateRequest[]) => void
}

interface ImportCandidate {
  id: string
  sourceFile: string
  name: string
  description: string
  server: string
  config: string
  selected: boolean
}

const isDesktopFilePickerAvailable = (): boolean =>
  typeof window !== 'undefined' && typeof window.kiteDesktop?.openFiles === 'function'

type ParsedKubeconfig = {
  'current-context'?: string
  contexts?: Array<{
    name?: string
    context?: {
      cluster?: string
    }
  }>
  clusters?: Array<{ name?: string; cluster?: { server?: string } }>
}

const getFileStem = (fileName: string): string => {
  const trimmed = fileName.trim()
  const dotIndex = trimmed.lastIndexOf('.')
  if (dotIndex <= 0) {
    return trimmed
  }
  return trimmed.slice(0, dotIndex)
}

const extractServerFromKubeconfig = (parsed: ParsedKubeconfig): string => {
  const clusters = Array.isArray(parsed.clusters) ? parsed.clusters : []
  if (clusters.length === 0) {
    return ''
  }

  const contexts = Array.isArray(parsed.contexts) ? parsed.contexts : []
  const currentContextName = (parsed['current-context'] || '').trim()
  if (currentContextName) {
    const currentContext = contexts.find((ctx) => ctx?.name === currentContextName)
    const currentClusterName = currentContext?.context?.cluster
    if (currentClusterName) {
      const currentCluster = clusters.find((cluster) => cluster?.name === currentClusterName)
      if (currentCluster?.cluster?.server) {
        return currentCluster.cluster.server
      }
    }
  }

  return clusters[0]?.cluster?.server || ''
}

export function ClusterDialog({
  open,
  onOpenChange,
  cluster,
  onSubmit,
}: ClusterDialogProps) {
  const { t } = useTranslation()
  const isEditMode = !!cluster
  const asciiClusterNameRegExp = /^[\x21-\x7E]+$/
  const [importCandidates, setImportCandidates] = useState<ImportCandidate[]>([])
  const [isPickingFiles, setIsPickingFiles] = useState(false)

  const [formData, setFormData] = useState({
    name: '',
    description: '',
    config: '',
    prometheusURL: '',
    enabled: true,
    isDefault: false,
    inCluster: false,
  })

  useEffect(() => {
    if (cluster) {
      setFormData({
        name: cluster.name,
        description: cluster.description || '',
        config: cluster.config || '',
        prometheusURL: cluster.prometheusURL || '',
        enabled: cluster.enabled,
        isDefault: cluster.isDefault,
        inCluster: cluster.inCluster,
      })
    }
  }, [cluster, open])

  useEffect(() => {
    if (!open || isEditMode) {
      return
    }
    setImportCandidates([])
  }, [open, isEditMode])

  const isClusterNameValid =
    formData.name === '' || asciiClusterNameRegExp.test(formData.name)
  const isImportMode = !isEditMode && importCandidates.length > 0

  const selectedImportCount = useMemo(
    () => importCandidates.filter((item) => item.selected).length,
    [importCandidates]
  )

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!isEditMode && selectedImportCount > 0) {
      const payload = importCandidates
        .filter((item) => item.selected)
        .map((item) => ({
          name: item.name.trim(),
          description: item.description.trim(),
          config: item.config,
          inCluster: false,
          enabled: true,
          isDefault: false,
        }))
        .filter((item) => item.name !== '' && asciiClusterNameRegExp.test(item.name))
      if (payload.length === 0) {
        return
      }
      onSubmit(payload)
      return
    }
    if (!isClusterNameValid) {
      return
    }
    onSubmit(formData)
  }

  const handleChange = (field: string, value: string | boolean) => {
    setFormData((prev) => ({
      ...prev,
      [field]: value,
    }))
  }

  const updateCandidate = (
    id: string,
    patch: Partial<Pick<ImportCandidate, 'name' | 'description' | 'selected'>>
  ) => {
    setImportCandidates((prev) =>
      prev.map((item) => (item.id === id ? { ...item, ...patch } : item))
    )
  }

  const removeCandidate = (id: string) => {
    setImportCandidates((prev) => prev.filter((item) => item.id !== id))
  }

  const parseKubeconfigCandidate = (
    fileName: string,
    content: string
  ): ImportCandidate | null => {
    let parsed: unknown
    try {
      parsed = yaml.load(content)
    } catch {
      return null
    }

    if (!parsed || typeof parsed !== 'object') {
      return null
    }

    const kubeconfig = parsed as ParsedKubeconfig
    const clusters = Array.isArray(kubeconfig.clusters) ? kubeconfig.clusters : []
    if (clusters.length === 0) {
      return null
    }

    const fallbackName = fileName.trim() || `cluster-${Date.now()}`
    const candidateName = getFileStem(fileName) || fallbackName

    return {
      id: `${fileName}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      sourceFile: fileName,
      name: candidateName,
      description: '',
      server: extractServerFromKubeconfig(kubeconfig),
      config: content,
      selected: true,
    }
  }

  const mergeImportCandidates = (nextCandidates: ImportCandidate[]) => {
    if (nextCandidates.length === 0) {
      return
    }
    setImportCandidates((prev) => {
      const existingNames = new Set(prev.map((item) => item.name))
      const deduped = nextCandidates.filter((item) => !existingNames.has(item.name))
      return [...prev, ...deduped]
    })
  }

  const handleImportLocalFiles = async () => {
    if (isDesktopFilePickerAvailable()) {
      setIsPickingFiles(true)
      try {
        const result = await window.kiteDesktop!.openFiles()
        if (!result || result.canceled || result.files.length === 0) {
          return
        }

        const nextCandidates: ImportCandidate[] = []
        const invalidFiles: string[] = []
        for (const file of result.files) {
          const parsedCandidate = parseKubeconfigCandidate(file.name, file.content)
          if (parsedCandidate) {
            nextCandidates.push(parsedCandidate)
          } else {
            invalidFiles.push(file.name)
          }
        }

        if (invalidFiles.length > 0) {
          toast.error(
            t(
              'clusterManagement.import.messages.invalidFilesSkipped',
              'Skipped {{count}} invalid kubeconfig files: {{files}}',
              {
                count: invalidFiles.length,
                files: invalidFiles.join(', '),
              }
            )
          )
        }
        if (nextCandidates.length === 0) {
          return
        }

        mergeImportCandidates(nextCandidates)
      } finally {
        setIsPickingFiles(false)
      }
      return
    }

    const input = document.createElement('input')
    input.type = 'file'
    input.multiple = true
    input.onchange = () => {
      const selectedFiles = Array.from(input.files || [])
      if (selectedFiles.length === 0) {
        return
      }
      void (async () => {
        const nextCandidates: ImportCandidate[] = []
        const invalidFiles: string[] = []
        for (const file of selectedFiles) {
          const content = await file.text()
          const parsedCandidate = parseKubeconfigCandidate(file.name, content)
          if (parsedCandidate) {
            nextCandidates.push(parsedCandidate)
          } else {
            invalidFiles.push(file.name)
          }
        }
        if (invalidFiles.length > 0) {
          toast.error(
            t(
              'clusterManagement.import.messages.invalidFilesSkipped',
              'Skipped {{count}} invalid kubeconfig files: {{files}}',
              {
                count: invalidFiles.length,
                files: invalidFiles.join(', '),
              }
            )
          )
        }
        if (nextCandidates.length === 0) {
          return
        }
        mergeImportCandidates(nextCandidates)
      })()
    }
    input.click()
  }

  const resetForm = () => {
    setFormData({
      name: '',
      description: '',
      config: '',
      prometheusURL: '',
      enabled: true,
      isDefault: false,
      inCluster: false,
    })
  }

  const handleOpenChange = (newOpen: boolean) => {
    onOpenChange(newOpen)
    if (!newOpen && !isEditMode) {
      // 关闭添加对话框时重置表单
      resetForm()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[1000px] sm:h-[88vh] sm:max-h-[88vh] overflow-hidden">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {isEditMode ? (
              <IconEdit className="h-5 w-5" />
            ) : (
              <IconServer className="h-5 w-5" />
            )}
            {isEditMode
              ? t('clusterManagement.dialog.edit.title', 'Edit Cluster')
              : t('clusterManagement.dialog.add.title', 'Add New Cluster')}
          </DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4 h-full overflow-y-auto pr-1">
          {!isEditMode && (
            <div className="rounded-lg border bg-muted/30 p-4 space-y-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <p className="text-sm font-medium">
                    {t(
                      'clusterManagement.import.title',
                      'Import from local files'
                    )}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t(
                      'clusterManagement.import.description',
                      'Select one or multiple local files. File extension is not restricted.'
                    )}
                  </p>
                </div>
                <Button
                  type="button"
                  variant="secondary"
                  className="gap-2"
                  onClick={() => {
                    void handleImportLocalFiles()
                  }}
                  disabled={isPickingFiles}
                >
                  <IconUpload className="h-4 w-4" />
                  {isPickingFiles
                    ? t('clusterManagement.import.choosing', 'Opening...')
                    : t('clusterManagement.import.chooseFiles', 'Choose Files')}
                </Button>
              </div>

              {importCandidates.length > 0 && (
                <>
                  <Separator />
                  <div className="flex items-center justify-between text-xs text-muted-foreground">
                    <span>
                      {t(
                        'clusterManagement.import.selectedCount',
                        '{{selected}} selected / {{total}} total',
                        {
                          selected: selectedImportCount,
                          total: importCandidates.length,
                        }
                      )}
                    </span>
                    <button
                      type="button"
                      className="underline underline-offset-2"
                      onClick={() => {
                        const allSelected =
                          importCandidates.length > 0 &&
                          importCandidates.every((item) => item.selected)
                        setImportCandidates((prev) =>
                          prev.map((item) => ({
                            ...item,
                            selected: !allSelected,
                          }))
                        )
                      }}
                    >
                      {t(
                        'clusterManagement.import.toggleSelection',
                        'Toggle selection'
                      )}
                    </button>
                  </div>
                  <div className="rounded-md border max-h-[40vh] overflow-auto">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead className="w-10"></TableHead>
                          <TableHead>
                            {t('clusterManagement.table.name', 'Name')}
                          </TableHead>
                          <TableHead>
                            {t(
                              'clusterManagement.import.table.description',
                              'Description'
                            )}
                          </TableHead>
                          <TableHead>
                            {t(
                              'clusterManagement.import.table.server',
                              'Server'
                            )}
                          </TableHead>
                          <TableHead>
                            {t(
                              'clusterManagement.import.table.source',
                              'Source File'
                            )}
                          </TableHead>
                          <TableHead className="w-14"></TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {importCandidates.map((item) => {
                          const validName = asciiClusterNameRegExp.test(item.name)
                          return (
                            <TableRow key={item.id}>
                              <TableCell>
                                <Checkbox
                                  checked={item.selected}
                                  onCheckedChange={(checked) =>
                                    updateCandidate(item.id, {
                                      selected: checked === true,
                                    })
                                  }
                                />
                              </TableCell>
                              <TableCell className="align-top whitespace-normal">
                                <div className="space-y-1">
                                  <Input
                                    value={item.name}
                                    onChange={(e) =>
                                      updateCandidate(item.id, {
                                        name: e.target.value,
                                      })
                                    }
                                  />
                                  {!validName && (
                                    <Badge variant="destructive">
                                      {t(
                                        'clusterManagement.form.name.asciiOnly',
                                        'Cluster name must use English/ASCII characters only. Do not use Chinese names.'
                                      )}
                                    </Badge>
                                  )}
                                </div>
                              </TableCell>
                              <TableCell className="align-top whitespace-normal">
                                <Input
                                  value={item.description}
                                  onChange={(e) =>
                                    updateCandidate(item.id, {
                                      description: e.target.value,
                                    })
                                  }
                                  placeholder={t(
                                    'clusterManagement.form.description.placeholder',
                                    'Brief description of this cluster'
                                  )}
                                />
                              </TableCell>
                              <TableCell className="align-top whitespace-normal">
                                <div
                                  className="max-w-[220px] truncate text-muted-foreground"
                                  title={item.server}
                                >
                                  {item.server || '-'}
                                </div>
                              </TableCell>
                              <TableCell className="align-top whitespace-normal">
                                <div
                                  className="max-w-[180px] truncate text-muted-foreground"
                                  title={item.sourceFile}
                                >
                                  {item.sourceFile}
                                </div>
                              </TableCell>
                              <TableCell className="align-top">
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="icon"
                                  onClick={() => removeCandidate(item.id)}
                                >
                                  <IconTrash className="h-4 w-4" />
                                </Button>
                              </TableCell>
                            </TableRow>
                          )
                        })}
                      </TableBody>
                    </Table>
                  </div>
                </>
              )}
            </div>
          )}

          {!isImportMode && (
            <>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="cluster-name">
                    {t('clusterManagement.form.name.label', 'Cluster Name')} *
                  </Label>
                  <Input
                    id="cluster-name"
                    value={formData.name}
                    onChange={(e) => handleChange('name', e.target.value)}
                    placeholder={t(
                      'clusterManagement.form.name.placeholder',
                      'e.g., production, staging'
                    )}
                    required
                    aria-invalid={!isClusterNameValid}
                  />
                  <p
                    className={`text-xs ${
                      isClusterNameValid
                        ? 'text-muted-foreground'
                        : 'text-destructive'
                    }`}
                  >
                    {t(
                      'clusterManagement.form.name.asciiOnly',
                      'Cluster name must use English/ASCII characters only. Do not use Chinese names.'
                    )}
                  </p>
                </div>

                {!isEditMode && (
                  <div className="space-y-2">
                    <Label htmlFor="cluster-type">
                      {t('clusterManagement.form.type.label', 'Cluster Type')}
                    </Label>
                    <Select
                      value={formData.inCluster ? 'inCluster' : 'external'}
                      onValueChange={(value) =>
                        handleChange('inCluster', value === 'inCluster')
                      }
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="external">
                          {t(
                            'clusterManagement.form.type.external',
                            'External Cluster'
                          )}
                        </SelectItem>
                        <SelectItem value="inCluster">
                          {t(
                            'clusterManagement.form.type.inCluster',
                            'In-Cluster'
                          )}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                )}
              </div>

              <div className="space-y-2">
                <Label htmlFor="cluster-description">
                  {t('clusterManagement.form.description.label', 'Description')}
                </Label>
                <Textarea
                  id="cluster-description"
                  value={formData.description}
                  onChange={(e) => handleChange('description', e.target.value)}
                  placeholder={t(
                    'clusterManagement.form.description.placeholder',
                    'Brief description of this cluster'
                  )}
                  rows={2}
                />
              </div>

              {!formData.inCluster && (
                <div className="space-y-2">
                  <Label htmlFor="cluster-config">
                    {t('clusterManagement.form.config.label', 'Kubeconfig')}
                    {!isEditMode && ' *'}
                  </Label>
                  {isEditMode && (
                    <p className="text-xs text-muted-foreground">
                      {t(
                        'clusterManagement.form.config.editNote',
                        'Leave empty to keep current configuration'
                      )}
                    </p>
                  )}
                  <Textarea
                    id="cluster-config"
                    value={formData.config}
                    onChange={(e) => handleChange('config', e.target.value)}
                    placeholder={t(
                      'clusterManagement.form.kubeconfig.placeholder',
                      'Paste your kubeconfig content here...'
                    )}
                    rows={8}
                    className="text-sm"
                    required={!isEditMode && !formData.inCluster}
                  />
                </div>
              )}

              <div className="space-y-2">
                <Label htmlFor="prometheus-url">
                  {t(
                    'clusterManagement.form.prometheusURL.label',
                    'Prometheus URL'
                  )}
                </Label>
                <Input
                  id="prometheus-url"
                  value={formData.prometheusURL}
                  onChange={(e) => handleChange('prometheusURL', e.target.value)}
                  type="url"
                />
              </div>

              {/* Cluster Status Controls */}
              <div className="space-y-4 border-t pt-4">
                {/* Enabled Status */}
                <div className="flex items-center justify-between">
                  <div className="space-y-1">
                    <Label htmlFor="cluster-enabled">
                      {t('clusterManagement.form.enabled.label', 'Enable Cluster')}
                    </Label>
                  </div>
                  <Switch
                    id="cluster-enabled"
                    checked={formData.enabled}
                    onCheckedChange={(checked) => handleChange('enabled', checked)}
                  />
                </div>

                {/* Default Status */}
                <div className="flex items-center justify-between">
                  <div className="space-y-1">
                    <Label htmlFor="cluster-default">
                      {t(
                        'clusterManagement.form.isDefault.label',
                        'Set as Default'
                      )}
                    </Label>
                    <p className="text-xs text-muted-foreground">
                      {t(
                        'clusterManagement.form.isDefault.help',
                        'Use this cluster as the default for new operations'
                      )}
                    </p>
                  </div>
                  <Switch
                    id="cluster-default"
                    checked={formData.isDefault}
                    onCheckedChange={(checked) =>
                      handleChange('isDefault', checked)
                    }
                  />
                </div>
              </div>

              {formData.inCluster && (
                <div className="p-4 bg-blue-50 dark:bg-blue-950/20 rounded-lg border border-blue-200 dark:border-blue-800">
                  <p className="text-sm text-blue-700 dark:text-blue-300">
                    {t(
                      'clusterManagement.form.inCluster.note',
                      'This cluster uses the in-cluster service account configuration. No additional kubeconfig is required.'
                    )}
                  </p>
                </div>
              )}
            </>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => handleOpenChange(false)}
            >
              {t('common.cancel', 'Cancel')}
            </Button>
            <Button
              type="submit"
              disabled={
                (isImportMode &&
                  (selectedImportCount === 0 ||
                    importCandidates
                      .filter((item) => item.selected)
                      .some(
                        (item) =>
                          item.name.trim() === '' ||
                          !asciiClusterNameRegExp.test(item.name)
                      ))) ||
                (!isImportMode &&
                  !isEditMode &&
                  (!formData.name ||
                    !isClusterNameValid ||
                    (!formData.inCluster && !formData.config)))
              }
            >
              {isEditMode
                ? t('clusterManagement.actions.save', 'Save Changes')
                : isImportMode
                  ? t(
                      'clusterManagement.import.importSelected',
                      'Import Selected'
                    )
                  : t('clusterManagement.actions.add', 'Add Cluster')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
