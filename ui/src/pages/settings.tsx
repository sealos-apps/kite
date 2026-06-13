import { useTranslation } from 'react-i18next'

import { usePageTitle } from '@/hooks/use-page-title'
import { GeneralManagement } from '@/components/settings/general-management'

export function SettingsPage() {
  const { t } = useTranslation()

  usePageTitle('Settings')

  return (
    <div className="space-y-2">
      <div className="mb-4">
        <div className="flex items-center gap-3 mb-2">
          <h1 className="text-3xl">
            {t('settings.tabs.aiAgent', 'AI Agent')}
          </h1>
        </div>
        <p className="text-muted-foreground">
          {t(
            'settings.description',
            'Manage AI Agent availability and model endpoint'
          )}
        </p>
      </div>

      <GeneralManagement />
    </div>
  )
}
