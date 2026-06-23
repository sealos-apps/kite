import { useMemo, useState } from 'react'
import { FileText } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { TextViewer } from '@/components/text-viewer'

export interface YamlFileTreeItem {
  path: string
  content: string
}

export type YamlDiffTreeItemStatus =
  | 'added'
  | 'deleted'
  | 'changed'
  | 'unchanged'

export interface YamlDiffTreeItem {
  path: string
  originalContent: string
  modifiedContent: string
  status: YamlDiffTreeItemStatus
}

function statusLabel(status?: YamlDiffTreeItemStatus) {
  switch (status) {
    case 'added':
      return 'A'
    case 'deleted':
      return 'D'
    case 'changed':
      return 'M'
    case 'unchanged':
      return '='
    default:
      return null
  }
}

function TreeShell<T extends { path: string }>({
  files,
  title,
  emptyMessage,
  fillHeight,
  getSearchText,
  renderContent,
  getStatus,
}: {
  files: T[]
  title: string
  emptyMessage: string
  fillHeight?: boolean
  getSearchText: (file: T) => string
  renderContent: (file: T, fillHeight?: boolean) => React.ReactNode
  getStatus?: (file: T) => YamlDiffTreeItemStatus | undefined
}) {
  const [searchQuery, setSearchQuery] = useState('')
  const visibleFiles = useMemo(() => {
    const query = searchQuery.trim().toLowerCase()
    if (!query) return files
    return files.filter((file) => getSearchText(file).toLowerCase().includes(query))
  }, [files, getSearchText, searchQuery])
  const [selectedPath, setSelectedPath] = useState('')
  const selectedFile =
    visibleFiles.find((file) => file.path === selectedPath) || visibleFiles[0]

  if (files.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{title}</CardTitle>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">
          {emptyMessage}
        </CardContent>
      </Card>
    )
  }

  return (
    <div
      className={cn(
        'grid min-h-0 gap-4 lg:grid-cols-[minmax(16rem,0.32fr)_minmax(0,1fr)]',
        fillHeight && 'h-full min-h-0 flex-1 overflow-hidden'
      )}
    >
      <Card className="flex min-h-0 flex-col gap-0 overflow-hidden rounded-lg border-border/70 py-0 shadow-none">
        <CardHeader className="shrink-0 px-3 py-2 !pb-2">
          <CardTitle className="text-balance text-sm">{title}</CardTitle>
        </CardHeader>
        <CardContent className="flex min-h-0 flex-1 flex-col gap-2 px-2 pb-2 pt-0">
          <Input
            aria-label="Search files"
            value={searchQuery}
            onChange={(event) => setSearchQuery(event.target.value)}
            placeholder="Search"
            className="h-8"
          />
          <div
            className={cn(
              'overflow-y-auto rounded-md bg-card px-2 py-1 text-[13px]',
              fillHeight ? 'min-h-0 flex-1' : 'h-[calc(100dvh-350px)] min-h-72'
            )}
          >
            {visibleFiles.map((file) => {
              const status = getStatus?.(file)
              const label = statusLabel(status)
              const selected = file.path === selectedFile?.path
              return (
                <button
                  key={file.path}
                  type="button"
                  className={cn(
                    'flex h-[30px] w-full min-w-0 items-center rounded-md px-2 text-left outline-none transition-colors hover:bg-muted/70 focus-visible:ring-1 focus-visible:ring-ring',
                    selected && 'bg-accent text-accent-foreground'
                  )}
                  onClick={() => setSelectedPath(file.path)}
                >
                  <FileText className="mr-2 size-3.5 shrink-0 text-muted-foreground" />
                  <span className="min-w-0 flex-1 truncate">{file.path}</span>
                  {label ? (
                    <span className="ml-2 shrink-0 text-[10px] font-semibold">
                      {label}
                    </span>
                  ) : null}
                </button>
              )
            })}
          </div>
        </CardContent>
      </Card>
      {selectedFile ? (
        renderContent(selectedFile, fillHeight)
      ) : (
        <Card>
          <CardContent className="pt-6 text-sm text-muted-foreground">
            {emptyMessage}
          </CardContent>
        </Card>
      )}
    </div>
  )
}

export function YamlFileTreeViewerNative({
  files,
  title,
  emptyMessage,
  fillHeight,
}: {
  files: YamlFileTreeItem[]
  title: string
  emptyMessage: string
  fillHeight?: boolean
}) {
  return (
    <TreeShell
      files={files}
      title={title}
      emptyMessage={emptyMessage}
      fillHeight={fillHeight}
      getSearchText={(file) => `${file.path}\n${file.content}`}
      renderContent={(file) => (
        <TextViewer value={file.content} title={file.path} />
      )}
    />
  )
}

export function YamlFileTreeDiffViewerNative({
  files,
  title,
  emptyMessage,
  fillHeight,
}: {
  files: YamlDiffTreeItem[]
  title: string
  emptyMessage: string
  fillHeight?: boolean
}) {
  return (
    <TreeShell
      files={files}
      title={title}
      emptyMessage={emptyMessage}
      fillHeight={fillHeight}
      getStatus={(file) => file.status}
      getSearchText={(file) =>
        `${file.path}\n${file.status}\n${file.originalContent}\n${file.modifiedContent}`
      }
      renderContent={(file) => (
        <div className="grid gap-4 xl:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center justify-between gap-2">
                <span className="truncate">{file.path}</span>
                <Badge variant="outline">{file.status}</Badge>
              </CardTitle>
            </CardHeader>
            <CardContent>
              <TextViewer value={file.originalContent} title="Original" />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="truncate">{file.path}</CardTitle>
            </CardHeader>
            <CardContent>
              <TextViewer value={file.modifiedContent} title="Modified" />
            </CardContent>
          </Card>
        </div>
      )}
    />
  )
}
