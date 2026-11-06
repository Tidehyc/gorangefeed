/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        // Orange is the product colour: the "GOrange" in GoRangeFeed.
        brand: {
          DEFAULT: '#FF5C35',
          soft: '#FF7C5C',
          deep: '#E0431F',
        },
        ink: {
          900: '#08090B',
          800: '#0E1013',
          700: '#14171C',
          600: '#1C2027',
          500: '#2A2F38',
          400: '#3A4049',
        },
        mist: {
          100: '#F4F5F7',
          300: '#C6CAD1',
          500: '#8E949E',
          700: '#5C626C',
        },
      },
      fontFamily: {
        sans: [
          '-apple-system',
          'BlinkMacSystemFont',
          '"Segoe UI"',
          '"PingFang SC"',
          '"Hiragino Sans GB"',
          '"Microsoft YaHei"',
          'sans-serif',
        ],
      },
      boxShadow: {
        float: '0 12px 40px rgba(0, 0, 0, 0.45)',
      },
      keyframes: {
        'heart-pop': {
          '0%': { transform: 'scale(0.4)', opacity: '0' },
          '35%': { transform: 'scale(1.15)', opacity: '1' },
          '60%': { transform: 'scale(0.95)', opacity: '1' },
          '100%': { transform: 'scale(1.6)', opacity: '0' },
        },
        'slide-up': {
          from: { transform: 'translateY(100%)' },
          to: { transform: 'translateY(0)' },
        },
        'fade-in': {
          from: { opacity: '0' },
          to: { opacity: '1' },
        },
        shimmer: {
          '100%': { transform: 'translateX(100%)' },
        },
      },
      animation: {
        'heart-pop': 'heart-pop 900ms ease-out forwards',
        'slide-up': 'slide-up 240ms cubic-bezier(0.22, 1, 0.36, 1)',
        'fade-in': 'fade-in 180ms ease-out',
        shimmer: 'shimmer 1.6s infinite',
      },
    },
  },
  plugins: [],
}
