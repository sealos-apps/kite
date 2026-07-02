import { useTranslation } from 'react-i18next'

import type { HelmReleaseDryRunResponse } from '@/types/api'
import { Badge } from '@/components/ui/badge'

export function HelmOfflineImageCheckNotice({
  imageCheck,
}: {
  imageCheck?: HelmReleaseDryRunResponse['imageCheck']
}) {
  const { t } = useTranslation()
  if (!imageCheck?.enabled) {
    return null
  }
  const imageCount = imageCheck.allImages?.length || 0
  const externalCount = imageCheck.externalImages?.length || 0
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md border border-border/70 bg-muted/40 px-3 py-2 text-sm">
      <Badge variant={externalCount ? 'destructive' : 'outline'}>
        {externalCount
          ? t('helm.messages.externalImages', { count: externalCount })
          : t('helm.messages.offlineImagesReady')}
      </Badge>
      <span className="text-muted-foreground">
        {imageCheck.registry
          ? t('helm.messages.offlineImageRegistry', {
              registry: imageCheck.registry,
            })
          : t('helm.messages.offlineImageRegistryEnabled')}
      </span>
      <span className="text-muted-foreground">
        {t('helm.messages.renderedImages', { count: imageCount })}
      </span>
      {imageCheck.injectedValues ? (
        <span className="text-muted-foreground">
          {t('helm.messages.imageRegistryInjected')}
        </span>
      ) : null}
    </div>
  )
}
