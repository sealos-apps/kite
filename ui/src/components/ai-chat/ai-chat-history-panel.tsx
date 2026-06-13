import { Clock, MessageSquarePlus, Trash2, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { ChatSession } from './ai-chat-types'

function formatRelativeDate(
  timestamp: number,
  t: ReturnType<typeof useTranslation>['t']
) {
  const date = new Date(timestamp)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMins = Math.floor(diffMs / 60000)
  const diffHours = Math.floor(diffMs / 3600000)
  const diffDays = Math.floor(diffMs / 86400000)

  if (diffMins < 1) return t('aiChat.messages.justNow')
  if (diffMins < 60) return t('aiChat.messages.minutesAgo', { count: diffMins })
  if (diffHours < 24) return t('aiChat.messages.hoursAgo', { count: diffHours })
  if (diffDays < 7) return t('aiChat.messages.daysAgo', { count: diffDays })
  return date.toLocaleDateString()
}

export function AIChatHistoryPanel({
  history,
  currentSessionId,
  onLoadSession,
  onDeleteSession,
  onNewSession,
  onClose,
}: {
  history: ChatSession[]
  currentSessionId: string | null
  onLoadSession: (id: string) => void
  onDeleteSession: (id: string) => void
  onNewSession: () => void
  onClose: () => void
}) {
  const { t } = useTranslation()

  return (
    <div className="absolute inset-0 z-20 flex flex-col bg-background">
      <div className="flex h-11 shrink-0 items-center justify-between border-b bg-muted/50 px-3">
        <div className="flex items-center gap-2 text-sm font-semibold">
          <Clock className="h-4 w-4" />
          {t('common.fields.chatHistory', 'Chat History')}
        </div>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7"
          onClick={onClose}
        >
          <X className="h-4 w-4" />
        </Button>
      </div>

      <div className="shrink-0 border-b p-2">
        <Button
          variant="outline"
          className="w-full justify-start gap-2"
          onClick={() => {
            onNewSession()
            onClose()
          }}
        >
          <MessageSquarePlus className="h-4 w-4" />
          {t('aiChat.actions.newChat')}
        </Button>
      </div>

      <div className="flex-1 overflow-y-auto">
        {history.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 p-8 text-center">
            <Clock className="h-8 w-8 text-muted-foreground" />
            <p className="text-sm text-muted-foreground">
              {t('common.messages.noChatHistory', 'No chat history yet')}
            </p>
          </div>
        ) : (
          <div className="space-y-1 p-2">
            {history.map((session) => (
              <div
                key={session.id}
                className={`group relative rounded-md border p-2 transition-colors hover:bg-muted ${
                  currentSessionId === session.id
                    ? 'border-primary bg-muted'
                    : 'border-transparent'
                }`}
              >
                <button
                  className="w-full text-left"
                  onClick={() => {
                    onLoadSession(session.id)
                    onClose()
                  }}
                >
                  <div className="mb-1 line-clamp-2 text-sm font-medium">
                    {session.title}
                  </div>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <span>{formatRelativeDate(session.updatedAt, t)}</span>
                    <span>•</span>
                    <span>
                      {t('aiChat.messages.count', {
                        count: session.messages.length,
                      })}
                    </span>
                    {session.clusterName && (
                      <>
                        <span>•</span>
                        <span className="truncate">{session.clusterName}</span>
                      </>
                    )}
                  </div>
                </button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="absolute right-1 top-1 h-6 w-6 opacity-0 group-hover:opacity-100 hover:bg-destructive hover:text-destructive-foreground"
                  onClick={(e) => {
                    e.stopPropagation()
                    onDeleteSession(session.id)
                  }}
                >
                  <Trash2 className="h-3 w-3" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
