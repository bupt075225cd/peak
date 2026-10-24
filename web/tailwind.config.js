/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,ts,tsx}'],
  theme: {
    extend: {
      colors: {
        primary: {
          DEFAULT: '#2563EB',
          light: '#3B82F6',
          lighter: '#60A5FA',
        },
        surface: {
          DEFAULT: '#FFFFFF',
          muted: '#F8FAFC',
          tint: '#EFF6FF',
        },
        ink: {
          DEFAULT: '#0F172A',
          soft: '#475569',
          faint: '#94A3B8',
        },
        // 家庭资产账本：低饱和金融配色（资产涨红、负债绿，沿用参考图语义）。
        ledger: {
          ink: '#1E2A44',
          mist: '#EEF0F6',
          haze: '#E6EAF3',
          up: '#D95C4A',
          down: '#3E9E7E',
          liquid: '#E9A66B',
          fixed: '#9DBCC4',
          invest: '#A9A3D4',
          receivable: '#63C6A5',
          liability: '#B9B4C7',
        },
      },
      fontFamily: {
        sans: ['PingFang SC', 'system-ui', 'sans-serif'],
        serif: ['"Noto Serif SC"', '"Songti SC"', 'STSong', 'SimSun', 'serif'],
      },
      keyframes: {
        'fade-up': {
          '0%': { opacity: '0', transform: 'translateY(8px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
        float: {
          '0%, 100%': { transform: 'translateY(0)' },
          '50%': { transform: 'translateY(-6px)' },
        },
      },
      animation: {
        'fade-up': 'fade-up 0.4s ease-out',
        float: 'float 3s ease-in-out infinite',
      },
    },
  },
  plugins: [require('tailwindcss-animate')],
}
