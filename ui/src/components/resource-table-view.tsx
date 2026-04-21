import React from 'react'
import {
  flexRender,
  PaginationState,
  Table as TableInstance,
} from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

interface ResourceTableViewProps<T> {
  table: TableInstance<T>
  columnCount: number
  isLoading: boolean
  data?: T[]
  allPageSize?: number
  maxBodyHeightClassName?: string
  containerClassName?: string
  emptyState: React.ReactNode
  hasActiveFilters: boolean
  filteredRowCount: number
  totalRowCount: number
  searchQuery: string
  pagination: PaginationState
  setPagination: React.Dispatch<React.SetStateAction<PaginationState>>
}

export function ResourceTableView<T>({
  table,
  columnCount,
  isLoading,
  data,
  allPageSize,
  maxBodyHeightClassName = 'max-h-[calc(100vh-210px)]',
  containerClassName = 'flex flex-col gap-3',
  emptyState,
  hasActiveFilters,
  filteredRowCount,
  totalRowCount,
  searchQuery,
  pagination,
  setPagination,
}: ResourceTableViewProps<T>) {
  const { t } = useTranslation()

  const renderRows = () => {
    const rows = table.getRowModel().rows

    if (rows.length === 0) {
      return (
        <TableRow>
          <TableCell colSpan={columnCount} className="h-24 text-center">
            {t('resourceTable.noResults')}
          </TableCell>
        </TableRow>
      )
    }

    return rows.map((row) => (
      <TableRow key={row.id} data-state={row.getIsSelected() && 'selected'}>
        {row.getVisibleCells().map((cell, index) => (
          <TableCell
            key={cell.id}
            className={`align-middle ${index <= 1 ? 'text-left' : 'text-center'}`}
          >
            {cell.column.columnDef.cell
              ? flexRender(cell.column.columnDef.cell, cell.getContext())
              : String(cell.getValue() || '-')}
          </TableCell>
        ))}
      </TableRow>
    ))
  }

  const dataLength = data?.length ?? 0
  const resolvedAllPageSize = allPageSize ?? dataLength

  return (
    <div className={containerClassName}>
      <div className="rounded-lg border overflow-hidden">
        <div
          className={`transition-opacity duration-200 ${
            isLoading && dataLength > 0 ? 'opacity-75' : 'opacity-100'
          }`}
        >
          {emptyState || (
            <div
              className={`relative ${maxBodyHeightClassName} overflow-auto scrollbar-hide`}
            >
              <Table>
                <TableHeader className="bg-muted">
                  {table.getHeaderGroups().map((headerGroup) => (
                    <TableRow key={headerGroup.id}>
                      {headerGroup.headers.map((header, index) => (
                        <TableHead
                          key={header.id}
                          className={index <= 1 ? 'text-left' : 'text-center'}
                        >
                          {header.isPlaceholder ? null : header.column.getCanSort() ? (
                            <Button
                              variant="ghost"
                              onClick={header.column.getToggleSortingHandler()}
                              className={
                                header.column.getIsSorted()
                                  ? 'text-primary'
                                  : ''
                              }
                            >
                              {flexRender(
                                header.column.columnDef.header,
                                header.getContext()
                              )}
                              {header.column.getIsSorted() && (
                                <span className="ml-2">
                                  {header.column.getIsSorted() === 'asc'
                                    ? '↑'
                                    : '↓'}
                                </span>
                              )}
                            </Button>
                          ) : (
                            flexRender(
                              header.column.columnDef.header,
                              header.getContext()
                            )
                          )}
                        </TableHead>
                      ))}
                    </TableRow>
                  ))}
                </TableHeader>
                <TableBody className="**:data-[slot=table-cell]:first:w-0">
                  {renderRows()}
                </TableBody>
              </Table>
            </div>
          )}
        </div>
      </div>

      {dataLength > 0 && (
        <div className="flex items-center justify-between px-2 py-1">
          <div className="text-muted-foreground hidden flex-1 text-sm lg:flex">
            {hasActiveFilters ? (
              <>
                {t('resourceTable.showingFilteredRows', {
                  filtered: filteredRowCount,
                  total: totalRowCount,
                })}
                {searchQuery && (
                  <span className="ml-1">
                    {t('resourceTable.filteredByQuery', {
                      query: searchQuery,
                    })}
                  </span>
                )}
              </>
            ) : (
              t('resourceTable.rowsTotal', { count: totalRowCount })
            )}
          </div>
          <div className="flex w-full items-center gap-4 lg:w-fit">
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">
                {t('resourceTable.rowsPerPage')}
              </span>
              <Select
                value={pagination.pageSize.toString()}
                onValueChange={(value) => {
                  setPagination((prev) => ({
                    ...prev,
                    pageSize: Number(value),
                    pageIndex: 0,
                  }))
                }}
              >
                <SelectTrigger size="sm" className="w-20" id="rows-per-page">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {[10, 20, 50, 100].map((pageSize) => (
                    <SelectItem key={pageSize} value={`${pageSize}`}>
                      {pageSize}
                    </SelectItem>
                  ))}
                  {resolvedAllPageSize > 0 && (
                    <SelectItem value={`${resolvedAllPageSize}`}>
                      {t('common.all')}
                    </SelectItem>
                  )}
                </SelectContent>
              </Select>
            </div>
            <div className="flex w-fit items-center justify-center text-sm font-medium">
              {t('resourceTable.pageOf', {
                page: pagination.pageIndex + 1,
                total: table.getPageCount() || 1,
              })}
            </div>
            <div className="ml-auto flex items-center gap-2 lg:ml-0">
              <Button
                variant="outline"
                className="size-8"
                size="icon"
                onClick={() => table.previousPage()}
                disabled={!table.getCanPreviousPage()}
              >
                <span className="sr-only">
                  {t('resourceTable.goToPreviousPage')}
                </span>
                ←
              </Button>
              <Button
                variant="outline"
                className="size-8"
                size="icon"
                onClick={() => table.nextPage()}
                disabled={!table.getCanNextPage()}
              >
                <span className="sr-only">
                  {t('resourceTable.goToNextPage')}
                </span>
                →
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
