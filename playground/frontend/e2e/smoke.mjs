// Дымовой e2e: открыть playground, дождаться примера, нажать «Взглянуть»,
// дождаться карточек, потрогать hover и little-endian, снять скриншоты.
//
//   node e2e/smoke.mjs                  # против localhost:8080
//   BASE=http://localhost:8092 SHOTS=/tmp/shots node e2e/smoke.mjs
//
// Требует запущенного сервера (go run ../backend) и chromium playwright
// (npx playwright install chromium, либо PW_CHROMIUM=/путь/к/chromium).
import { chromium } from 'playwright'
import { mkdirSync } from 'node:fs'

const base = process.env.BASE ?? 'http://localhost:8080'
const shots = process.env.SHOTS ?? 'e2e/shots'
mkdirSync(shots, { recursive: true })

const browser = await chromium.launch({
  executablePath: process.env.PW_CHROMIUM || undefined,
})
const page = await browser.newPage({ viewport: { width: 1600, height: 950 } })
let failed = false
page.on('pageerror', (e) => {
  failed = true
  console.error('pageerror:', e.message)
})

await page.goto(base)
await page.waitForSelector('.monaco-editor', { timeout: 20000 })
await page.waitForFunction(() => document.querySelector('select')?.value !== '', {
  timeout: 10000,
})
await page.screenshot({ path: `${shots}/01_loaded.png` })

await page.click('button:has-text("Взглянуть")')
await page.waitForSelector('article', { timeout: 30000 })
await page.screenshot({ path: `${shots}/02_result.png`, fullPage: true })

const field = page.locator('article ul li').first()
await field.hover()
await page.waitForTimeout(300)
await page.screenshot({ path: `${shots}/03_hover.png` })

await field.click()
await page.waitForTimeout(700)
await page.screenshot({ path: `${shots}/04_little_endian.png` })

await page.selectOption('select', '05_interfaces')
await page.click('button:has-text("Взглянуть")')
await page.waitForSelector('section:has-text("интерфейс")', { timeout: 30000 })
await page.waitForTimeout(500)
await page.screenshot({ path: `${shots}/05_ifaces.png`, fullPage: true })

await browser.close()
if (failed) {
  console.error('e2e: на странице были ошибки')
  process.exit(1)
}
console.log('e2e ok — скриншоты в', shots)
