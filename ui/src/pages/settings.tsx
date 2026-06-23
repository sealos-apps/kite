import { useAuth } from '@/contexts/auth-context'
import { IconRobot } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { usePageTitle } from '@/hooks/use-page-title'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { GeneralManagement } from '@/components/settings/general-management'

export function SettingsPage() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const isAdmin = user?.isAdmin() ?? false

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

      {isAdmin ? (
        <GeneralManagement />
      ) : (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <IconRobot className="h-5 w-5" />
              {t('settings.tabs.aiAgent', 'AI Agent')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm leading-6 text-muted-foreground">
              {t(
                'settings.aiAgentRequiresAdmin',
                'AI Agent settings are managed by Kite administrators. Ask an administrator to configure the API key and model endpoint before using chat.'
              )}
            </p>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
