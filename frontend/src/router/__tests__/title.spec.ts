import { describe, expect, it } from 'vitest'
import { resolveDocumentTitle } from '@/router/title'

describe('resolveDocumentTitle', () => {
  it('固定返回品牌标签"芝麻灵码"，忽略路由/站点名参数', () => {
    expect(resolveDocumentTitle('Usage Records', 'My Site')).toBe('芝麻灵码')
    expect(resolveDocumentTitle(undefined, '   ')).toBe('芝麻灵码')
    expect(resolveDocumentTitle()).toBe('芝麻灵码')
  })
})
