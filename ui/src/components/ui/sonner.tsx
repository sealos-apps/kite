import { useTheme } from 'next-themes'
import { Toaster as Sonner, ToasterProps } from 'sonner'

const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = 'system' } = useTheme()

  return (
    <Sonner
      theme={theme as ToasterProps['theme']}
      className="toaster group"
      style={
        {
          '--normal-bg': 'rgba(var(--popover), var(--popover-alpha, 1))',
          '--normal-text':
            'rgba(var(--popover-foreground), var(--popover-foreground-alpha, 1))',
          '--normal-border': 'rgba(var(--border), var(--border-alpha, 1))',
        } as React.CSSProperties
      }
      toastOptions={{
        classNames: {
          error: '[&_svg]:!text-red-500',
          success: '[&_svg]:!text-green-500',
          warning: '[&_svg]:!text-amber-500',
          info: '[&_svg]:!text-blue-500',
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
