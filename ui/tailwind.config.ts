import type { Config } from 'tailwindcss'
import animate from 'tailwindcss-animate'

const withAlpha =
  (variable: string) =>
  ({ opacityValue }: { opacityValue?: string }) => {
    const alphaVariable = `${variable}-alpha`

    if (opacityValue === undefined) {
      return `rgba(var(${variable}), var(${alphaVariable}, 1))`
    }

    return `rgba(var(${variable}), calc(var(${alphaVariable}, 1) * ${opacityValue}))`
  }

const config: Config = {
  darkMode: 'class',
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        background: withAlpha('--background'),
        foreground: withAlpha('--foreground'),
        card: {
          DEFAULT: withAlpha('--card'),
          foreground: withAlpha('--card-foreground'),
        },
        popover: {
          DEFAULT: withAlpha('--popover'),
          foreground: withAlpha('--popover-foreground'),
        },
        primary: {
          DEFAULT: withAlpha('--primary'),
          foreground: withAlpha('--primary-foreground'),
        },
        secondary: {
          DEFAULT: withAlpha('--secondary'),
          foreground: withAlpha('--secondary-foreground'),
        },
        muted: {
          DEFAULT: withAlpha('--muted'),
          foreground: withAlpha('--muted-foreground'),
        },
        accent: {
          DEFAULT: withAlpha('--accent'),
          foreground: withAlpha('--accent-foreground'),
        },
        destructive: {
          DEFAULT: withAlpha('--destructive'),
          foreground: withAlpha('--destructive-foreground'),
        },
        border: withAlpha('--border'),
        input: withAlpha('--input'),
        ring: withAlpha('--ring'),
        chart: {
          1: withAlpha('--chart-1'),
          2: withAlpha('--chart-2'),
          3: withAlpha('--chart-3'),
          4: withAlpha('--chart-4'),
          5: withAlpha('--chart-5'),
        },
        sidebar: {
          DEFAULT: withAlpha('--sidebar'),
          foreground: withAlpha('--sidebar-foreground'),
          primary: withAlpha('--sidebar-primary'),
          'primary-foreground': withAlpha('--sidebar-primary-foreground'),
          accent: withAlpha('--sidebar-accent'),
          'accent-foreground': withAlpha('--sidebar-accent-foreground'),
          border: withAlpha('--sidebar-border'),
          ring: withAlpha('--sidebar-ring'),
        },
      },
      borderRadius: {
        xs: 'calc(var(--radius) - 6px)',
        sm: 'calc(var(--radius) - 4px)',
        md: 'calc(var(--radius) - 2px)',
        lg: 'var(--radius)',
        xl: 'calc(var(--radius) + 4px)',
      },
      boxShadow: {
        xs: '0 1px 2px 0 rgba(0, 0, 0, 0.05)',
      },
    },
  },
  plugins: [animate],
}

export default config
