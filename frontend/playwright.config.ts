import {defineConfig} from '@playwright/test';
import {mkdtempSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

// End-to-end tests drive the real app (Go backend + this frontend) built in
// Wails "server" mode: the same UI and bindings, served over HTTP so a
// headless browser can use it. Each run gets a fresh, throwaway data folder.
const port = Number(process.env.E2E_PORT ?? 38917);
const dataHome = mkdtempSync(join(tmpdir(), 'romcure-e2e-'));
// Wails on Linux defaults to GTK4; the project targets GTK3 (see ci.yml).
const tags = process.platform === 'linux' ? 'server,production,gtk3' : 'server,production';
const exe = process.platform === 'win32' ? 'romcure-server.exe' : 'romcure-server';

export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  workers: 1,
  use: {baseURL: `http://127.0.0.1:${port}`, viewport: {width: 1100, height: 740}},
  webServer: {
    command: `npm run build && cd .. && go build -tags ${tags} -o bin/${exe} ./cmd/romcure && ${join('bin', exe)}`,
    url: `http://127.0.0.1:${port}/`,
    timeout: 240_000,
    reuseExistingServer: false,
    env: {
      WAILS_SERVER_PORT: String(port),
      // Pretend links are unavailable so the administrator offer is shown (never clicked here).
      ROMCURE_ASSUME_NO_SYMLINKS: '1',
      // Make short jobs last long enough to watch and cancel.
      ROMCURE_STEP_DELAY_MS: '300',
      // os.UserConfigDir() per OS: %AppData% on Windows, $XDG_CONFIG_HOME on
      // Linux, $HOME/Library/Application Support on macOS.
      APPDATA: dataHome,
      LOCALAPPDATA: dataHome, // os.UserCacheDir() on Windows: the default cache location
      XDG_CACHE_HOME: dataHome,
      XDG_CONFIG_HOME: dataHome,
      HOME: dataHome,
    },
  },
});
