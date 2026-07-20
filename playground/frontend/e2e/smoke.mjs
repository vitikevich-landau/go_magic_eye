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

// странствие: живой сеанс — дерево, раскрытия, прыжок по циклу
await page.selectOption('select', '07_explore')
await page.click('button:has-text("странствие")')
await page.click('button:has-text("Взглянуть")')
await page.waitForSelector('article', { timeout: 30000 }) // Гримуар корня
await page.screenshot({ path: `${shots}/06_explore_root.png` })

// раскрыть корень, затем Members → [0] → рыцарь; дойти до цикла Home
const expand = async (label) => {
  const row = page.locator('div.cursor-pointer', { hasText: label }).first()
  await row.locator('button').first().click()
  await page.waitForTimeout(250)
}
await expand('гильдия')
await expand('Members')
await page.waitForTimeout(300)
await page.screenshot({ path: `${shots}/07_explore_tree.png`, fullPage: true })

// найти узел с меткой цикла (⟲ или ≡) — он появляется глубже: раскрываем
// первого рыцаря
const knight = page.locator('div.cursor-pointer', { hasText: '[0]' }).first()
await knight.locator('button').first().click()
await page.waitForTimeout(400)
// [0] раскрывает узел-разыменование «цель», под ним — поля рыцаря
await expand('цель')
// Home — указатель назад к гильдии: раскрытие доводит до узла-цикла ⟲
await expand('Home')
const cycleBadge = page.locator('button:has-text("⟲"), button:has-text("≡")').first()
if ((await cycleBadge.count()) === 0) {
  console.error('e2e: узел-цикл ⟲/≡ не появился после раскрытия Home')
  failed = true
} else {
  await cycleBadge.click() // прыжок к оригиналу
  await page.waitForTimeout(400)
}
await page.screenshot({ path: `${shots}/08_explore_cycle.png`, fullPage: true })

// правка кода в странствии рвёт связь дерева с живой памятью — дерево
// должно исчезнуть (сеанс закрыт), сменившись подсказкой
await page.locator('.monaco-editor textarea').fill('package main\n\nfunc main() {}\n')
await page.waitForTimeout(300)
if ((await page.locator('section:has-text("Странствие — прогулка")').count()) === 0) {
  console.error('e2e: дерево не сброшено после правки кода в странствии')
  failed = true
}
await page.screenshot({ path: `${shots}/09_explore_reset.png` })

await browser.close()
if (failed) {
  console.error('e2e: на странице были ошибки')
  process.exit(1)
}
console.log('e2e ok — скриншоты в', shots)
