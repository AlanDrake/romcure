import {test, expect} from '@playwright/test';
import {mkdtempSync, writeFileSync, readFileSync, readdirSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

// One workflow, in order: the app's state lives in the server process, so
// the steps share it.
test.describe.configure({mode: 'serial'});

const romDir = mkdtempSync(join(tmpdir(), 'romcure-roms-'));
const outDir = join(mkdtempSync(join(tmpdir(), 'romcure-out-')), 'plain');
const cardDir = join(mkdtempSync(join(tmpdir(), 'romcure-card-')), 'card');
writeFileSync(join(romDir, 'Alpha (USA).nes'), 'alpha');
writeFileSync(join(romDir, 'Alpha (Europe).nes'), 'alpha-eu');
writeFileSync(join(romDir, 'Beta (USA).nes'), 'beta');

const tab = (page: any, name: string) => page.locator('nav').getByRole('button', {name, exact: true}).click();

test('first run: the setup finds the games folder and builds the first output', async ({page}) => {
  await page.goto('/');
  await expect(page.getByRole('heading', {name: 'Welcome to RomCure'})).toBeVisible();
  await page.getByRole('button', {name: 'Next'}).click();

  await page.getByLabel('Folder with your games').fill(romDir);
  await page.getByRole('button', {name: 'Look here'}).click();
  const found = page.locator('.found tbody tr');
  await expect(found).toHaveCount(1);
  await expect(found.getByLabel(/System of/)).toHaveValue('nes');
  await expect(found).toContainText('3'); // the three files
  await page.screenshot({path: 'test-results/setup-folders.png'});
  await page.getByRole('button', {name: 'Next'}).click();

  await expect(page.getByRole('heading', {name: 'Which versions do you prefer?'})).toBeVisible();
  await page.getByRole('button', {name: 'Next'}).click();

  await expect(page.getByRole('heading', {name: 'Where should your games go?'})).toBeVisible();
  await page.getByText('Somewhere else').click();
  await page.getByLabel('Output name').fill('Plain');
  await page.getByLabel('Kind of output').selectOption('raw');
  await page.getByLabel('Output folder', {exact: true}).fill(outDir);
  await page.screenshot({path: 'test-results/setup-output.png'});
  await page.getByRole('button', {name: 'Next'}).click();

  await expect(page.getByRole('heading', {name: 'Links'})).toBeVisible();
  await page.getByRole('button', {name: 'Next'}).click();
  await expect(page.getByRole('heading', {name: 'Where to keep converted games'})).toBeVisible();
  await expect(page.locator('#cache-path')).toContainText('Default:');
  await page.getByRole('button', {name: 'Next'}).click();
  await expect(page.getByRole('heading', {name: 'Game databases (optional)'})).toBeVisible();
  await page.getByRole('button', {name: 'Next'}).click();

  await expect(page.getByRole('heading', {name: 'Ready'})).toBeVisible();
  await page.getByRole('button', {name: 'Start first sync'}).click();
  await expect(page.getByText('Sync finished.')).toBeVisible({timeout: 60_000});
  expect(readdirSync(join(outDir, 'nes')).sort()).toEqual(['Alpha.nes', 'Beta.nes']);
});

test('after setup the app opens normally, and setup can be run again from Help', async ({page}) => {
  await page.goto('/');
  await expect(page.locator('nav')).toBeVisible();
  await expect(page.getByRole('heading', {name: 'Welcome to RomCure'})).toHaveCount(0);
  await tab(page, 'Help');
  await page.getByRole('button', {name: 'Run setup again'}).click();
  await expect(page.getByRole('heading', {name: 'Welcome to RomCure'})).toBeVisible();
  await page.getByRole('button', {name: 'Skip setup'}).click();
  await expect(page.locator('nav')).toBeVisible();
});

test('a system can have a second folder, and removing it again forgets it', async ({page}) => {
  const extra = mkdtempSync(join(tmpdir(), 'romcure-extra-'));
  writeFileSync(join(extra, 'Delta (USA).nes'), 'delta');
  await page.goto('/');
  await page.getByLabel('System', {exact: true}).selectOption('nes');
  await page.getByLabel('Folder path').fill(extra);
  await page.getByRole('button', {name: 'Add folder', exact: true}).click();
  const row = page.locator('ul.rows li', {hasText: extra});
  await expect(row).toBeVisible();
  await expect(page.locator('.msg.err')).toHaveCount(0);
  await row.getByRole('button', {name: 'Remove'}).click();
  await expect(row).toHaveCount(0);
});

test('sync picks one version per game and fills the output', async ({page}) => {
  await page.goto('/');
  await page.getByRole('button', {name: 'Sync now'}).click();
  await expect(page.getByText('Sync finished.')).toBeVisible({timeout: 30_000});
  // Default "USA first" preset: Alpha (USA) kept, Alpha (Europe) skipped.
  const row = page.locator('table tbody tr').first();
  await expect(row).toContainText('nes');
  await expect(row.locator('td').nth(1)).toHaveText('2');
  await expect(row.locator('td').nth(2)).toHaveText('1');

  await tab(page, 'Outputs');
  await expect(page.locator('.result')).toContainText('nes: 2 games');
  await expect(page.locator('.result .bad')).toHaveCount(0);

  const files = readdirSync(join(outDir, 'nes')).sort();
  expect(files).toEqual(['Alpha.nes', 'Beta.nes']);
});

test('settings are validated and saved', async ({page}) => {
  await page.goto('/');
  await tab(page, 'Settings');
  await page.locator('label', {hasText: 'Conversions at once'}).locator('input').fill('20');
  await page.getByRole('button', {name: 'Save settings'}).click();
  await expect(page.locator('.msg.err')).toContainText('parallelism');

  await page.locator('label', {hasText: 'Conversions at once'}).locator('input').fill('2');
  await page.locator('label', {hasText: 'Region preference'}).locator('select').selectOption('europe');
  await page.getByRole('button', {name: 'Save settings'}).click();
  await expect(page.getByText('Settings saved.')).toBeVisible();

  await page.reload();
  await tab(page, 'Settings');
  await expect(page.locator('label', {hasText: 'Region preference'}).locator('select')).toHaveValue('europe');
});

test('switching the preset changes the pick on the next sync', async ({page}) => {
  await page.goto('/');
  await page.getByRole('button', {name: 'Sync now'}).click();
  await expect(page.getByText('Sync finished.')).toBeVisible({timeout: 30_000});
  // Europe first now: the same output name must now lead to the European file.
  expect(readFileSync(join(outDir, 'nes', 'Alpha.nes'), 'utf8')).toBe('alpha-eu');
  await tab(page, 'Outputs');
  await expect(page.locator('.result .bad')).toHaveCount(0);
});


// ---- Games screen (docs/game-selection-plan.md phase 3)

const row = (page: any, name: string) => page.locator('.row', {hasText: name});
const applyButton = (page: any) => page.getByRole('button', {name: 'Apply to Plain'});

async function apply(page: any) {
  await applyButton(page).click();
  await expect(page.getByText('Outputs are up to date.')).toBeVisible({timeout: 30_000});
}

test('the games screen lists every game with its state on the output', async ({page}) => {
  await page.goto('/');
  await tab(page, 'Games');
  await expect(page.getByText('Choosing for')).toBeVisible();
  await expect(page.locator('.row')).toHaveCount(2);
  await expect(page.locator('.shown')).toContainText('2 shown · 2 included');
  await expect(page.locator('.rail .sys').nth(1)).toContainText('2/2');
  await expect(page.locator('.bar')).toContainText('Plain is up to date');
  await expect(row(page, 'Alpha')).toContainText('2 versions');
  await page.screenshot({path: 'test-results/games.png'});
});

test('the systems rail can be resized, and the width is remembered', async ({page}) => {
  await page.goto('/');
  await tab(page, 'Games');
  const sep = page.locator('.railsep');
  const before = (await page.locator('.rail').boundingBox())!.width;

  const box = (await sep.boundingBox())!;
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + 80, box.y + box.height / 2, {steps: 5});
  await page.mouse.up();
  const dragged = (await page.locator('.rail').boundingBox())!.width;
  expect(dragged).toBeGreaterThan(before + 60);

  // The width survives a reload...
  await page.reload();
  await tab(page, 'Games');
  const reloaded = (await page.locator('.rail').boundingBox())!.width;
  expect(reloaded).toBeCloseTo(dragged, 0);

  // ...and the keyboard widens it further from the handle.
  await sep.focus();
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  const widened = (await page.locator('.rail').boundingBox())!.width;
  expect(widened).toBeGreaterThan(reloaded);

  // Double-clicking resets it to the default.
  await sep.dblclick();
  const reset = (await page.locator('.rail').boundingBox())!.width;
  expect(reset).toBeCloseTo(before, 0);
});

test('search narrows the list; the several-versions filter works', async ({page}) => {
  await page.goto('/');
  await tab(page, 'Games');
  await page.getByLabel('Search games').fill('bet');
  await expect(page.locator('.shown')).toContainText('1 shown');
  await expect(page.locator('.row')).toHaveCount(1);
  await page.getByLabel('Search games').fill('zzz');
  await expect(page.locator('.empty')).toContainText('No games match');
  await page.getByRole('button', {name: 'Clear filters'}).first().click();
  await expect(page.locator('.row')).toHaveCount(2);
  await page.getByRole('button', {name: 'Several versions'}).click();
  await expect(page.locator('.shown')).toContainText('1 shown');
});

test('unticking a game changes the output only after Apply', async ({page}) => {
  await page.goto('/');
  await tab(page, 'Games');
  await row(page, 'Beta').locator('input').click();
  await expect(page.locator('.bar')).toContainText('−1 to remove');
  await expect(page.locator('.rail .sys').nth(1)).toContainText('1/2');
  expect(readdirSync(join(outDir, 'nes')).sort()).toEqual(['Alpha.nes', 'Beta.nes']);

  await apply(page);
  expect(readdirSync(join(outDir, 'nes')).sort()).toEqual(['Alpha.nes']);
  await expect(page.locator('.bar')).toContainText('Plain is up to date');

  // And back: tick it again, apply.
  await row(page, 'Beta').locator('input').click();
  await expect(page.locator('.bar')).toContainText('+1 to add');
  await apply(page);
  expect(readdirSync(join(outDir, 'nes')).sort()).toEqual(['Alpha.nes', 'Beta.nes']);
});

test('a system box acts on the whole system and the toast undoes it', async ({page}) => {
  await page.goto('/');
  await tab(page, 'Games');
  await page.getByLabel('Include every NES game in Plain').click();
  await expect(page.locator('.toast')).toContainText('Left out 2 games from Plain');
  await expect(page.locator('.bar')).toContainText('−2 to remove');
  await page.locator('.toast').getByRole('button', {name: 'Undo'}).click();
  await expect(page.locator('.bar')).toContainText('Plain is up to date');
  await expect(page.locator('.rail .sys').nth(1)).toContainText('2/2');
});

test('the header box acts on everything the filter shows', async ({page}) => {
  await page.goto('/');
  await tab(page, 'Games');
  await page.getByLabel('Search games').fill('alpha');
  await expect(page.locator('.shown')).toContainText('1 shown');
  await page.getByLabel(/Leave out all 1 shown/).click();
  await expect(page.locator('.rail .sys').nth(1)).toContainText('1/2');
  await page.getByRole('button', {name: 'Undo'}).first().click();
  await expect(page.locator('.rail .sys').nth(1)).toContainText('2/2');
});

test('choosing a version in the details marks the output and Apply uses it', async ({page}) => {
  await page.goto('/');
  await tab(page, 'Games');
  await row(page, 'Alpha').click();
  const drawer = page.locator('.drawer');
  await expect(drawer).toContainText('Version');
  await expect(drawer.locator('.ver.on')).toContainText('Alpha (Europe)');
  await page.screenshot({path: 'test-results/games-drawer.png'});

  await drawer.locator('.ver', {hasText: 'Alpha (USA)'}).click();
  await expect(page.locator('.bar')).toContainText('1 version change');
  await expect(row(page, 'Alpha')).toContainText('your version');
  expect(readFileSync(join(outDir, 'nes', 'Alpha.nes'), 'utf8')).toBe('alpha-eu'); // not before Apply
  await apply(page);
  expect(readFileSync(join(outDir, 'nes', 'Alpha.nes'), 'utf8')).toBe('alpha');

  await drawer.getByRole('button', {name: 'Back to automatic'}).click();
  await expect(page.locator('.bar')).toContainText('1 version change');
  await apply(page);
  expect(readFileSync(join(outDir, 'nes', 'Alpha.nes'), 'utf8')).toBe('alpha-eu');
});

test('the games screen fits a narrow window', async ({page}) => {
  await page.setViewportSize({width: 700, height: 740});
  await page.goto('/');
  await tab(page, 'Games');
  await expect(page.locator('.row')).toHaveCount(2);
  await page.screenshot({path: 'test-results/games-narrow.png'});
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow).toBe(0);
});

// ---- Outputs tab (phase 4)

test('an output can start with no games, and Choose games opens the screen on it', async ({page}) => {
  await page.goto('/');
  await tab(page, 'Outputs');
  await expect(page.locator('.card.out', {hasText: 'Plain'})).toContainText('2 of 2 games');
  await page.getByPlaceholder('Name, e.g. My RetroArch').fill('Card');
  await page.getByLabel('Kind of output').selectOption('raw');
  await page.getByLabel('Start with').selectOption('none');
  await page.getByLabel('Output folder path').fill(cardDir);
  await page.getByRole('button', {name: 'Add output'}).click();
  const card = page.locator('.card.out', {hasText: 'Card'});
  await expect(card).toContainText('0 of 2 games');
  await expect(card).toContainText('Up to date');
  await page.screenshot({path: 'test-results/outputs.png', fullPage: true});

  await card.getByRole('button', {name: 'Choose games…'}).click();
  await expect(page.locator('#out-pick')).toHaveText(/Card/);
  await expect(page.locator('.shown')).toContainText('2 shown · 0 included');
  await row(page, 'Alpha').locator('input').click();
  await expect(page.locator('.bar')).toContainText('+1 to add');
  await page.getByRole('button', {name: 'Apply to Card'}).click();
  await expect(page.getByText('Outputs are up to date.')).toBeVisible({timeout: 30_000});
  expect(readdirSync(join(cardDir, 'nes')).sort()).toEqual(['Alpha.nes']);
});

test('the Outputs tab shows waiting changes and applies them', async ({page}) => {
  await page.goto('/');
  await tab(page, 'Games');
  await page.locator('#out-pick').selectOption({label: 'Card'});
  await row(page, 'Beta').locator('input').click();
  await tab(page, 'Outputs');
  const card = page.locator('.card.out', {hasText: 'Card'});
  await expect(card).toContainText('+1 to add');
  await card.getByRole('button', {name: 'Apply', exact: true}).click();
  await expect(card).toContainText('Up to date', {timeout: 30_000});
  expect(readdirSync(join(cardDir, 'nes')).sort()).toEqual(['Alpha.nes', 'Beta.nes']);
});

test('the sync report links to the games of a system', async ({page}) => {
  await page.goto('/');
  await page.locator('table').getByRole('button', {name: 'nes'}).click();
  await expect(page.locator('.sys.on')).toContainText('NES');
  await expect(page.locator('.row')).toHaveCount(2);
});

test('removing an output can delete the links it made', async ({page}) => {
  await page.goto('/');
  await tab(page, 'Outputs');
  const card = page.locator('.card.out', {hasText: 'Card'});
  await card.getByRole('button', {name: 'Remove'}).click();
  await card.getByRole('button', {name: 'Delete links'}).click();
  await expect(page.locator('.card.out', {hasText: 'Card'})).toHaveCount(0);
  expect(readdirSync(join(cardDir, 'nes'))).toEqual([]);
});

// ---- Elevation setting (bug 1)

test('the administrator offer can be dismissed for good', async ({page}) => {
  await page.goto('/');
  await expect(page.locator('#link-offer')).toContainText('Links need administrator rights');
  await page.screenshot({path: 'test-results/link-offer.png'});
  await page.getByRole('button', {name: 'Keep using copies'}).click();
  await expect(page.locator('#link-offer')).toHaveCount(0);
  await page.reload();
  await expect(page.locator('#link-offer')).toHaveCount(0);
  await tab(page, 'Settings');
  await expect(page.locator('label', {hasText: 'When Windows needs administrator rights'}).locator('select')).toHaveValue('never');
});

// ---- Progress strip (docs/ux-roadmap.md section 3)

test('a running job shows its progress on every tab and can be cancelled', async ({page}) => {
  await page.goto('/');
  await page.getByRole('button', {name: 'Sync now'}).click();
  const strip = page.locator('.strip');
  await expect(strip).toContainText('Syncing');
  await expect(strip).toContainText(/Reading your NES folder|Identifying NES games|Updating/);
  await expect(strip.locator('.time')).toHaveText(/\d+:\d\d/);
  await expect(strip.locator('.count')).toHaveText(/^\d+\/\d+$/);
  await page.screenshot({path: 'test-results/progress-strip.png'});
  // It follows you to other tabs.
  await tab(page, 'Games');
  await expect(strip).toContainText('Syncing');
  await strip.getByRole('button', {name: 'Cancel'}).click();
  await expect(page.getByText('Sync cancelled.')).toBeVisible({timeout: 15_000});
  await expect(strip).toHaveCount(0);
});

test('a finished sync links to its report', async ({page}) => {
  await page.goto('/');
  await page.getByRole('button', {name: 'Sync now'}).click();
  await expect(page.getByText('Sync finished.')).toBeVisible({timeout: 30_000});
  await tab(page, 'Settings');
  await page.getByRole('button', {name: 'See the report'}).click();
  await expect(page.getByRole('heading', {name: 'Your ROM folders'})).toBeVisible();
});

// ---- A damaged archive must not stop the sync

test('a damaged archive is skipped and listed while everything else syncs', async ({page}) => {
  const bad = join(romDir, 'Broken (USA).7z');
  writeFileSync(bad, 'not really a 7z archive');
  writeFileSync(join(romDir, 'Omega (USA).nes'), 'omega');
  await page.goto('/');
  await page.getByRole('button', {name: 'Sync now'}).click();
  await expect(page.locator('.msg.warn')).toContainText('could not be read and was skipped', {timeout: 60_000});
  const list = page.locator('details.unreadable');
  await expect(list).toContainText('1 file could not be read');
  await expect(list).toContainText('Broken (USA).7z');
  await page.screenshot({path: 'test-results/unreadable.png'});
  // "See which" lands on the list from any tab.
  await tab(page, 'Settings');
  await page.getByRole('button', {name: 'See which'}).click();
  await expect(list).toBeInViewport();
  // The good game after it was still picked up.
  await tab(page, 'Games');
  await expect(row(page, 'Omega')).toBeVisible();
  // Put things back so later runs start clean.
  rmSync(bad);
  rmSync(join(romDir, 'Omega (USA).nes'));
  await tab(page, 'Library');
  await page.getByRole('button', {name: 'Sync now'}).click();
  await expect(page.getByText('Sync finished.')).toBeVisible({timeout: 60_000});
});

// ---- Backlog items: codec fields, edit an output, check and repair

test('the CHD codec settings are validated and saved', async ({page}) => {
  await page.goto('/');
  await tab(page, 'Settings');
  await page.getByLabel('CD codecs').fill('bogus');
  await page.getByRole('button', {name: 'Save settings'}).click();
  await expect(page.locator('.msg.err')).toContainText('unknown codec');
  await page.getByLabel('CD codecs').fill('cdlz,cdzl');
  await page.getByRole('button', {name: 'Save settings'}).click();
  await expect(page.getByText('Settings saved.')).toBeVisible();
  await page.reload();
  await tab(page, 'Settings');
  await expect(page.getByLabel('CD codecs')).toHaveValue('cdlz,cdzl');
});

test('an output can be renamed, and its missing files checked and repaired', async ({page}) => {
  await page.goto('/');
  await tab(page, 'Outputs');
  const card = page.locator('.card.out', {hasText: 'Plain'});
  await card.getByRole('button', {name: 'Edit'}).click();
  await page.getByLabel('Output name').fill('Plain files');
  await page.getByRole('button', {name: 'Save', exact: true}).click();
  await expect(page.locator('.card.out', {hasText: 'Plain files'})).toBeVisible();

  rmSync(join(outDir, 'nes', 'Beta.nes'));
  const renamed = page.locator('.card.out', {hasText: 'Plain files'});
  await renamed.getByRole('button', {name: 'Check files'}).click();
  await expect(renamed.locator('.problems')).toContainText('Beta.nes');
  await page.screenshot({path: 'test-results/outputs-problems.png', fullPage: true});
  await renamed.getByRole('button', {name: 'Repair'}).click();
  await expect(page.getByText('Outputs are up to date.')).toBeVisible({timeout: 30_000});
  expect(readdirSync(join(outDir, 'nes')).sort()).toEqual(['Alpha.nes', 'Beta.nes']);
  await renamed.getByRole('button', {name: 'Check files'}).click();
  await expect(renamed.locator('.problems')).toContainText('Every file is where it should be');
});

// ---- Third-party licenses and the source offer (spec section 7)

test('the Help tab lists third-party licenses with their text, and the source offer', async ({page}) => {
  await page.goto('/');
  await tab(page, 'Help');
  const licenses = page.locator('ul.licenses');
  await expect(page.getByRole('heading', {name: 'Third-party licenses'})).toBeVisible();
  await expect(licenses.first()).toContainText('RomCure');
  const chdman = page.locator('ul.licenses li', {hasText: 'chdman (MAME)'});
  await expect(chdman).toContainText('GPL-2.0-or-later');
  await chdman.getByRole('button', {name: 'Show license'}).click();
  await expect(chdman.locator('.license-text')).toContainText('GNU GENERAL PUBLIC LICENSE');
  await chdman.getByRole('button', {name: 'Hide license'}).click();
  await expect(chdman.locator('.license-text')).toHaveCount(0);
  await page.getByRole('button', {name: /Show all \d+ libraries/}).click();
  await expect(page.locator('ul.licenses li', {hasText: 'modernc.org/sqlite'})).toContainText('BSD-3-Clause');
  await expect(page.getByRole('heading', {name: 'Source code'})).toBeVisible();
  await expect(page.getByText('own unmodified releases, and their source is published by them')).toBeVisible();
  await page.screenshot({path: 'test-results/licenses.png', fullPage: true});
});

test('a No-Intro DAT can be imported by hand, its system found from its name', async ({page}) => {
  const dat = join(mkdtempSync(join(tmpdir(), 'romcure-dat-')), 'nes.dat');
  writeFileSync(dat, `<?xml version="1.0"?>
<datafile><header><name>Nintendo - Nintendo Entertainment System (Headered) (Parent-Clone)</name></header>
<game name="Alpha (USA)"><rom name="Alpha (USA).nes" size="5" crc="12345678"/></game>
</datafile>`);
  await page.goto('/');
  await page.getByLabel('DAT file').fill(dat);
  await page.getByRole('button', {name: 'Import DAT'}).click();
  await expect(page.locator('.msg.ok')).toContainText('for Nintendo - Nintendo Entertainment System: 1 game.');
  await expect(page.locator('.msg.ok')).toContainText('Press Sync now');

  await page.getByLabel('DAT file').fill(join(tmpdir(), 'no-such-file.dat'));
  await page.getByRole('button', {name: 'Import DAT'}).click();
  await expect(page.locator('.msg.err')).toBeVisible();
});

// ---- "See the results" after Update DATs / Check CHD files

test('a finished CHD check links to its results, right under the button', async ({page}) => {
  // Update DATs goes online, so the CHD check (an empty disc folder still gets a row) stands in for both.
  await page.goto('/');
  const check = page.getByRole('button', {name: 'Check CHD files'});
  test.skip((await check.getAttribute('title')) === 'chdman was not found', 'needs chdman (tools/fetch-tools.sh)');
  const discs = mkdtempSync(join(tmpdir(), 'romcure-discs-'));
  await page.getByLabel('System', {exact: true}).selectOption('psx');
  await page.getByLabel('Folder path').fill(discs);
  await page.getByRole('button', {name: 'Add folder', exact: true}).click();
  const source = page.locator('ul.rows li', {hasText: discs});
  await expect(source).toBeVisible();

  await check.click();
  await expect(page.locator('.msg.ok')).toContainText('CHD check finished.', {timeout: 30_000});
  const results = page.locator('#chd-results');
  await expect(results).toContainText('0 CHD files checked');
  // Still on the Library tab: the link has to scroll, not just switch tabs.
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect(results).not.toBeInViewport();
  await page.getByRole('button', {name: 'See the results'}).click();
  await expect(results).toBeInViewport();
  const importHeading = page.getByRole('heading', {name: 'Import a DAT file'});
  expect((await results.boundingBox())!.y).toBeLessThan((await importHeading.boundingBox())!.y);
  await page.screenshot({path: 'test-results/chd-results.png'});

  // Put things back so later runs start clean.
  await source.getByRole('button', {name: 'Remove'}).click();
  await expect(source).toHaveCount(0);
});

// ---- Choosing which RetroArch an output is for

test('a RetroArch output asks which RetroArch it is for, other kinds do not', async ({page}) => {
  await page.goto('/');
  await page.getByRole('button', {name: 'Outputs', exact: true}).click();
  const picker = page.getByText('Which RetroArch is this output for?');
  await expect(picker).toBeVisible();
  await expect(page.getByRole('radio', {name: /Not now/})).toBeVisible();
  await expect(page.getByRole('button', {name: 'Choose RetroArch…'})).toBeVisible();
  await picker.scrollIntoViewIfNeeded();
  await page.screenshot({path: 'test-results/retroarch-picker.png'});
  await page.getByLabel('Kind of output').selectOption('esde');
  await expect(picker).toHaveCount(0);
});
