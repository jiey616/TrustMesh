import type { Config } from 'tailwindcss'

export default {
  // Prefix all Tailwind classes with "tw-" to avoid conflicts with antd
  prefix: 'tw-',
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // Dopamine palette — vibrant sci-fi
        canvas: '#080812',
        'canvas-deep': '#050508',
        surface: {
          1: '#0d0d1a',
          2: '#111124',
          3: '#16162e',
          4: '#1c1c38',
        },
        neon: {
          purple: '#a855f7',
          'purple-bright': '#c084fc',
          blue: '#3b82f6',
          'blue-bright': '#60a5fa',
          cyan: '#22d3ee',
          'cyan-bright': '#67e8f9',
          rose: '#f43f5e',
          'rose-bright': '#fb7185',
          amber: '#f59e0b',
          'amber-bright': '#fbbf24',
          green: '#10b981',
          'green-bright': '#34d399',
        },
        glass: {
          base: 'rgba(255,255,255,0.04)',
          elevated: 'rgba(255,255,255,0.06)',
          border: 'rgba(255,255,255,0.08)',
          'border-bright': 'rgba(255,255,255,0.12)',
        },
      },
      fontFamily: {
        sans: ['Inter', '-apple-system', 'BlinkMacSystemFont', "'Segoe UI'", 'Roboto', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'],
      },
      borderRadius: {
        glass: '16px',
        'glass-lg': '20px',
      },
      backdropBlur: {
        glass: '24px',
        'glass-heavy': '40px',
      },
      boxShadow: {
        'neon-purple': '0 0 20px rgba(168,85,247,0.25), 0 0 60px rgba(168,85,247,0.1)',
        'neon-cyan': '0 0 20px rgba(34,211,238,0.25), 0 0 60px rgba(34,211,238,0.1)',
        'neon-blue': '0 0 20px rgba(59,130,246,0.25), 0 0 60px rgba(59,130,246,0.1)',
        'glass': '0 8px 32px rgba(0,0,0,0.4)',
        'glass-sm': '0 4px 16px rgba(0,0,0,0.3)',
      },
      animation: {
        'float': 'float 6s ease-in-out infinite',
        'float-delayed': 'float 6s ease-in-out 2s infinite',
        'float-slow': 'float 8s ease-in-out 1s infinite',
        'aurora': 'aurora 8s ease infinite',
        'glow-pulse': 'glowPulse 3s ease-in-out infinite',
        'beam': 'beam 4s linear infinite',
        'border-glow': 'borderGlow 3s ease-in-out infinite',
        'slide-up': 'slideUp 0.5s ease-out',
        'fade-in': 'fadeIn 0.4s ease-out',
        'gradient-flow': 'gradientFlow 4s ease infinite',
      },
      keyframes: {
        float: {
          '0%, 100%': { transform: 'translateY(0px) rotate(0deg)' },
          '33%': { transform: 'translateY(-10px) rotate(1deg)' },
          '66%': { transform: 'translateY(5px) rotate(-1deg)' },
        },
        aurora: {
          '0%, 100%': { backgroundPosition: '0% 50%' },
          '50%': { backgroundPosition: '100% 50%' },
        },
        glowPulse: {
          '0%, 100%': { opacity: '0.4' },
          '50%': { opacity: '1' },
        },
        beam: {
          '0%': { transform: 'translateX(-100%)' },
          '100%': { transform: 'translateX(200%)' },
        },
        borderGlow: {
          '0%, 100%': { borderColor: 'rgba(168,85,247,0.2)' },
          '50%': { borderColor: 'rgba(168,85,247,0.5)' },
        },
        slideUp: {
          from: { opacity: '0', transform: 'translateY(12px)' },
          to: { opacity: '1', transform: 'translateY(0)' },
        },
        fadeIn: {
          from: { opacity: '0' },
          to: { opacity: '1' },
        },
        gradientFlow: {
          '0%': { backgroundPosition: '0% 50%' },
          '50%': { backgroundPosition: '100% 50%' },
          '100%': { backgroundPosition: '0% 50%' },
        },
      },
    },
  },
  plugins: [],
} satisfies Config
