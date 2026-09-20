import { describe, expect, it } from 'vitest'
import { DEFAULT_API_BASE, resolveApiBase } from './resolveApiBase'

describe('resolveApiBase', () => {
  it('无输入时回落到同源相对地址', () => {
    expect(resolveApiBase()).toBe('/api/v1/')
    expect(resolveApiBase({ override: null, envBase: null })).toBe(DEFAULT_API_BASE)
  })

  it('环境变量生效并补尾斜杠', () => {
    expect(resolveApiBase({ envBase: 'https://175.27.135.91/api/v1' })).toBe(
      'https://175.27.135.91/api/v1/',
    )
  })

  it('手动配置优先级高于环境变量（现场可换服，无需重新打包）', () => {
    expect(
      resolveApiBase({ override: 'https://10.0.0.9/api/v1', envBase: 'https://175.27.135.91/api/v1' }),
    ).toBe('https://10.0.0.9/api/v1/')
  })

  it('忽略空白与多余尾斜杠', () => {
    expect(resolveApiBase({ override: '   ' })).toBe(DEFAULT_API_BASE)
    expect(resolveApiBase({ override: 'https://a.example.com///' })).toBe('https://a.example.com/')
  })
})
