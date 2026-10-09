import test from 'node:test'
import assert from 'node:assert/strict'
import {
  buildAddMessage,
  cookieHeader,
  hostMatches,
  pickPageUrl,
  knownSize,
  normalizeHost,
  parseHostList,
  shouldCapture,
} from '../lib/capture.js'
import { classifyError } from '../lib/native.js'

const base = { enabled: true, minSizeMB: 1, excludedHosts: [] }
const item = (over = {}) => ({ url: 'https://example.com/big.zip', state: 'in_progress', totalBytes: 50e6, ...over })

test('captures a large http(s) download', () => {
  assert.equal(shouldCapture(item(), base).capture, true)
  assert.equal(shouldCapture(item({ url: 'http://example.com/a.iso' }), base).capture, true)
})

test('respects the master switch', () => {
  const r = shouldCapture(item(), { ...base, enabled: false })
  assert.deepEqual(r, { capture: false, reason: 'disabled' })
})

test('ignores non-http schemes', () => {
  for (const url of ['blob:https://x/uuid', 'data:text/plain,hi', 'file:///tmp/a.zip', 'ftp://x/a.zip', 'not a url']) {
    assert.equal(shouldCapture(item({ url }), base).capture, false, url)
  }
})

test('ignores extension-initiated and finished downloads', () => {
  assert.equal(shouldCapture(item({ byExtensionId: 'abc' }), base).capture, false)
  assert.equal(shouldCapture(item({ state: 'complete' }), base).capture, false)
  assert.equal(shouldCapture(item({ state: 'interrupted' }), base).capture, false)
})

test('minimum size: small skipped, unknown captured', () => {
  assert.equal(shouldCapture(item({ totalBytes: 200 * 1024 }), base).capture, false)
  assert.equal(shouldCapture(item({ totalBytes: -1 }), base).capture, true)
  assert.equal(shouldCapture(item({ totalBytes: 200 * 1024 }), { ...base, minSizeMB: 0 }).capture, true)
})

test('exclusion list matches host and subdomains only', () => {
  const s = { ...base, excludedHosts: ['example.com'] }
  assert.equal(shouldCapture(item({ url: 'https://example.com/a.zip' }), s).capture, false)
  assert.equal(shouldCapture(item({ url: 'https://cdn.example.com/a.zip' }), s).capture, false)
  assert.equal(shouldCapture(item({ url: 'https://notexample.com/a.zip' }), s).capture, true)
})

test('host list parsing', () => {
  assert.equal(normalizeHost(' HTTPS://Www.Example.com:8080/path?q '), 'www.example.com')
  assert.equal(normalizeHost('*.example.com'), 'example.com')
  assert.deepEqual(parseHostList('a.com\n\nA.com, b.org\n'), ['a.com', 'b.org'])
  assert.equal(hostMatches('a.b.example.com', 'example.com'), true)
})

test('knownSize prefers totalBytes then fileSize', () => {
  assert.equal(knownSize({ totalBytes: 10, fileSize: 5 }), 10)
  assert.equal(knownSize({ totalBytes: -1, fileSize: 5 }), 5)
  assert.equal(knownSize({ totalBytes: -1, fileSize: 0 }), -1)
})

test('cookie header and message building', () => {
  assert.equal(cookieHeader([{ name: 'a', value: '1' }, { name: 'b', value: '2' }]), 'a=1; b=2')
  assert.equal(cookieHeader([]), '')
  assert.deepEqual(buildAddMessage({ url: 'https://x/a.zip', size: -1 }), { type: 'add', url: 'https://x/a.zip' })
  assert.deepEqual(
    buildAddMessage({ url: 'u', referrer: 'r', cookies: 'a=1', userAgent: 'UA', mime: 'm', size: 9 }),
    { type: 'add', url: 'u', referrer: 'r', cookies: 'a=1', userAgent: 'UA', mime: 'm', size: 9 },
  )
})

test('native error classification', () => {
  assert.equal(classifyError('Specified native messaging host not found.'), 'host_not_installed')
  assert.equal(classifyError('Access to the specified native messaging host is forbidden.'), 'host_forbidden')
  assert.equal(classifyError('Native host has exited.'), 'host_exited')
})

test('native error classification, Firefox wording', () => {
  assert.equal(classifyError('No such native application com.goidm.host'), 'host_not_installed')
  assert.equal(
    classifyError('This extension does not have permission to use native application com.goidm.host'),
    'host_forbidden',
  )
})

test('page url: a full referrer is the page', () => {
  assert.equal(
    pickPageUrl('https://site.example/dl/page?id=3', [{ url: 'https://site.example/other', active: true }]),
    'https://site.example/dl/page?id=3',
  )
})

test('page url: an origin-only referrer is upgraded to an open tab on the same site', () => {
  const tab = (url, over = {}) => ({ url, active: false, lastAccessed: 0, ...over })
  const tabs = [tab('https://site.example/dl/page', { lastAccessed: 5 }), tab('https://other.example/x', { lastAccessed: 9 })]
  assert.equal(pickPageUrl('https://site.example/', tabs), 'https://site.example/dl/page')
  assert.equal(pickPageUrl('https://site.example/', [tab('https://other.example/x')]), 'https://site.example/')
  assert.equal(pickPageUrl('https://site.example/', []), 'https://site.example/')
  assert.equal(pickPageUrl('https://site.example/'), 'https://site.example/')
})

test('page url: the file opening its own tab does not hide the page', () => {
  // gofile.io: the page is in a background tab, the active tab is the file host.
  const tabs = [
    { url: 'https://gofile.io/d/G2T4JXGI', active: false, lastAccessed: 100 },
    { url: 'https://store3.gofile.io/download/x/file.zip', active: true, lastAccessed: 200 },
  ]
  assert.equal(pickPageUrl('https://gofile.io/', tabs), 'https://gofile.io/d/G2T4JXGI')
})

test('page url: among same-site tabs the most recently used wins', () => {
  const tabs = [
    { url: 'https://site.example/old', lastAccessed: 1 },
    { url: 'https://site.example/new', lastAccessed: 7 },
  ]
  assert.equal(pickPageUrl('https://site.example/', tabs), 'https://site.example/new')
})

test('page url: nothing usable gives an empty string', () => {
  assert.equal(pickPageUrl('', [{ url: 'https://site.example/page' }]), '')
  assert.equal(pickPageUrl(undefined, undefined), '')
  assert.equal(pickPageUrl('about:blank', [{ url: 'https://site.example/page' }]), '')
  assert.equal(pickPageUrl('chrome://downloads', []), '')
})

test('add message carries the page url only when there is one', () => {
  assert.equal(buildAddMessage({ url: 'https://x/a.zip', pageUrl: 'https://x/p' }).pageUrl, 'https://x/p')
  assert.equal('pageUrl' in buildAddMessage({ url: 'https://x/a.zip', pageUrl: '' }), false)
})
