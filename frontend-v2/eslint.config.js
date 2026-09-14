import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  // 只忽略 'dist' 是不够的：本项目构建产物目录实际是 dist-v2 / dist-v2-c3 / dist-prod，
  // 另有 electron-builder 的 release / release-new 与 build-output。
  // eslint 默认连 .js 一起 lint，于是它会去扫打包后的单个巨型 bundle：
  // 耗时从数秒涨到 3 分半，并报出上千条假错误（bundle 里残留的
  // /* eslint-disable react-hooks/exhaustive-deps */ 注释引用了未加载的规则，
  // 变成 "Definition for rule ... was not found"）。这里把产物目录一并排除。
  globalIgnores([
    '**/node_modules/**',
    '**/dist*/**', // dist / dist-v2 / dist-v2-c3 / dist-prod
    '**/release*/**', // release / release-new
    '**/build-output/**',
    '**/.vite-cache*/**',
    '**/tmp/**',
  ]),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
  },
])
