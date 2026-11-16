import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

test('应用清单来自接口并提供静态快照兜底', async () => {
  const source = await readFile(new URL('../src/App.vue', import.meta.url), 'utf8')
  assert.ok(source.includes('loadApps(token.value)'))
  assert.ok(source.includes('/_pagehub/catalog.json'))
  assert.match(source, /state:\s*["']unknown["']/)
})

test('前端构建产物不保存管理员凭据', async () => {
  const source = await readFile(new URL('../src/App.vue', import.meta.url), 'utf8')
  assert.ok(!source.includes('localStorage'))
  assert.ok(!source.includes('sessionStorage'))
})
