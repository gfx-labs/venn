/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./templ/**/*.templ",
    "./templates/**/*.go",
  ],
  theme: {
    extend: {
      colors: {
        gray: {
          900: '#0f0f0f',
          800: '#1a1a1a',
          700: '#2a2a2a',
          600: '#3a3a3a',
          500: '#5a5a5a',
          400: '#7a7a7a',
          300: '#9a9a9a',
          200: '#bababa',
          100: '#dadada',
        }
      }
    },
  },
}