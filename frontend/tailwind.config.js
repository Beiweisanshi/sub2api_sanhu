/**
 * 作者：mkx
 * 日期：2026-04-22
 * 变更说明：A+B 和谐化 - primary 降饱和至陶土橙、gray 换暖灰、claude.* 对齐 gray 数值、glow 改纸本阴影
 */
/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        primary: {
          50: '#FBF2EC',
          100: '#F4DED1',
          200: '#E8C3AF',
          300: '#DCA589',
          400: '#CE8564',
          500: '#C96442',
          600: '#B15434',
          700: '#934328',
          800: '#76361F',
          900: '#5A2917',
          950: '#2B120B'
        },
        claude: {
          bg: '#FAF8F4',
          text: '#2D2A26',
          muted: '#756F62',
          accent: '#C96442',
          border: '#E5E0D6',
          card: '#FFFFFF',
          code: '#1E1E1E',
          'code-bg': '#F4EFE6'
        },
        gray: {
          50: '#FAF8F4',
          100: '#F2EEE6',
          200: '#E5E0D6',
          300: '#CFC8BA',
          400: '#A39C8E',
          500: '#756F62',
          600: '#5A554A',
          700: '#433F37',
          800: '#2D2A26',
          900: '#1F1C18',
          950: '#0F0D0A'
        },
        // 语义色别名：方便 @apply 时直接引用语义名
        // 作者：mkx  日期：2026-04-22
        success: {
          50: '#ECFDF5',
          100: '#D1FAE5',
          200: '#A7F3D0',
          300: '#6EE7B7',
          400: '#34D399',
          500: '#10B981',
          600: '#059669',
          700: '#047857',
          800: '#065F46',
          900: '#064E3B',
          950: '#022C22'
        },
        info: {
          50: '#F0F9FF',
          100: '#E0F2FE',
          200: '#BAE6FD',
          300: '#7DD3FC',
          400: '#38BDF8',
          500: '#0EA5E9',
          600: '#0284C7',
          700: '#0369A1',
          800: '#075985',
          900: '#0C4A6E',
          950: '#082F49'
        }
      },
      fontFamily: {
        sans: [
          'Inter',
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        serif: ['"Noto Serif SC"', 'Georgia', 'serif'],
        mono: ['"JetBrains Mono"', 'ui-monospace', 'Menlo', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: '0 8px 32px rgba(0, 0, 0, 0.08)',
        'glass-sm': '0 4px 16px rgba(0, 0, 0, 0.06)',
        glow: '0 1px 2px rgba(201, 100, 66, 0.06), 0 1px 3px rgba(45, 42, 38, 0.04)',
        'glow-lg': '0 2px 6px rgba(201, 100, 66, 0.08), 0 1px 2px rgba(45, 42, 38, 0.05)',
        card: '0 1px 3px rgba(0, 0, 0, 0.04), 0 1px 2px rgba(0, 0, 0, 0.06)',
        'card-hover': '0 10px 40px rgba(0, 0, 0, 0.08)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.1)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #C96442 0%, #B15434 100%)',
        'gradient-dark': 'linear-gradient(135deg, #37332E 0%, #2D2A26 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.1) 0%, rgba(255,255,255,0.05) 100%)',
        'mesh-gradient':
          'radial-gradient(at 40% 20%, rgba(201, 100, 66, 0.06) 0px, transparent 50%), radial-gradient(at 80% 0%, rgba(117, 111, 98, 0.05) 0px, transparent 50%), radial-gradient(at 0% 50%, rgba(201, 100, 66, 0.05) 0px, transparent 50%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 1px 2px rgba(201, 100, 66, 0.05)' },
          '100%': { boxShadow: '0 2px 6px rgba(201, 100, 66, 0.08)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
