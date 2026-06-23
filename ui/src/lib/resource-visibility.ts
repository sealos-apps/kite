export const HIDDEN_RESOURCE_TYPES = ['gateways', 'httproutes'] as const
export const HIDDEN_SIDEBAR_PATHS = ['/settings'] as const

const hiddenResourceTypeSet = new Set<string>(HIDDEN_RESOURCE_TYPES)
const hiddenSidebarPathSet = new Set<string>(HIDDEN_SIDEBAR_PATHS)

export const isResourceTypeHidden = (resourceType?: string | null): boolean => {
  if (!resourceType) {
    return false
  }
  return hiddenResourceTypeSet.has(resourceType.toLowerCase())
}

export const isSidebarPathHidden = (path?: string | null): boolean => {
  if (!path) {
    return false
  }

  const pathWithoutQuery = path.split('?')[0]
  if (hiddenSidebarPathSet.has(pathWithoutQuery)) {
    return true
  }

  const firstSegment = pathWithoutQuery.replace(/^\/+/, '').split('/')[0]
  return isResourceTypeHidden(firstSegment)
}
