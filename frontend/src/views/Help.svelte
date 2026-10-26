<script lang="ts">
  import {onMount} from 'svelte';
  import type {LicenseInfo} from '../../bindings/romcure/cmd/romcure/models';
  import * as Service from '../../bindings/romcure/cmd/romcure/service';
  import {app, fail, ok} from '../lib/state.svelte';

  // Third-party licenses (spec §7)
  let licenses: LicenseInfo[] = $state([]);
  let texts: Record<string, string> = $state({});
  let showAll = $state(false);
  const kindTitle: Record<string, string> = {
    app: 'This application',
    tool: 'Bundled programs (run as separate programs)',
    go: 'Libraries compiled into the app',
    js: 'Interface components',
    font: 'Fonts',
    data: 'Data downloaded on request',
  };
  const kindOrder = ['app', 'tool', 'go', 'js', 'font', 'data'];
  onMount(async () => {
    try {
      licenses = (await Service.Licenses()) ?? [];
    } catch (e) {
      fail(e);
    }
  });
  async function toggleText(name: string) {
    if (texts[name] !== undefined) {
      delete texts[name];
      return;
    }
    try {
      texts[name] = await Service.LicenseText(name);
    } catch (e) {
      fail(e);
    }
  }
  const byKind = (k: string) => licenses.filter((l) => l.Kind === k);

  async function exportLogs() {
    try {
      const where = await Service.ExportLogBundle();
      if (where) ok('Diagnostics saved to ' + where);
    } catch (e) {
      fail(e);
    }
  }
</script>

{#if app.st}
  {@const st = app.st}
  <section>
    <h2>How it works</h2>
    <div class="card">
      <h3>Links, not copies</h3>
      <p>
        Outputs point at your original files with <strong>symbolic links</strong>, so a library on a big drive is
        available to several programs without using extra space. If links can't be made, the app tells you why and
        uses the next best method (a space-saving clone, or finally a copy).
      </p>
      <h3>Windows and links</h3>
      <p>
        Making symbolic links on Windows needs either Developer Mode or administrator rights. The app starts
        normally and asks only when it needs them; saying no is fine, it will copy instead.
      </p>
      <h3>SD cards and USB sticks</h3>
      <p>
        FAT32/exFAT drives can't hold links, so use the <strong>SD card / USB (copy)</strong> output. It checks free
        space first, flags files over FAT32's 4 GB limit, and only copies what changed next time.
      </p>
      <h3>Disc games</h3>
      <p>
        PlayStation, PSP and other disc games can be shrunk to CHD, and GameCube/Wii to RVZ, with a check that the
        result is identical. The converted files live in a cache the outputs link to.
      </p>
    </div>

    <h2>Setup</h2>
    <button onclick={() => (app.setupOpen = true)}>Run setup again</button>
    <span class="muted">Walks through folders, versions and outputs. Nothing already set up is removed.</span>

    <h2>Tools</h2>
    <ul class="rows">
      {#each st.Tools ?? [] as t}
        <li>
          <div><strong>{t.Name}</strong> <span class={t.Found ? 'good' : 'bad'}>{t.Found ? 'found' : 'not found'}</span>
            {#if t.Path}<span class="path">{t.Path}</span>{/if}</div>
        </li>
      {/each}
    </ul>
    <p class="muted">Not found? Put the program in the <b>tools</b> folder next to RomCure, or install it so it is on your PATH (for example <b>mame-tools</b> or <b>rom-tools</b> for chdman, <b>dolphin-emu</b> for dolphin-tool, <b>7zip</b> for RAR files).</p>

    <h2>Third-party licenses</h2>
    <p class="muted">RomCure is free software (GPL-3.0). It includes the components below, and runs the converters as separate programs.</p>
    {#each kindOrder as k}
      {#if byKind(k).length}
        <h3>{kindTitle[k]}</h3>
        <ul class="rows licenses">
          {#each (k === 'go' && !showAll ? byKind(k).slice(0, 8) : byKind(k)) as l (l.Name)}
            <li>
              <div>
                <strong>{l.Name}</strong>
                {#if l.Version}<span class="tag">{l.Version}</span>{/if}
                <span class="tag">{l.License}</span>
                <a href={l.URL} target="_blank" rel="noreferrer">website</a>
                <button class="link" onclick={() => toggleText(l.Name)}>{texts[l.Name] !== undefined ? 'Hide license' : 'Show license'}</button>
                {#if l.Note}<span class="detail">{l.Note}</span>{/if}
                {#if texts[l.Name] !== undefined}<pre class="license-text">{texts[l.Name]}</pre>{/if}
              </div>
            </li>
          {/each}
        </ul>
        {#if k === 'go' && byKind(k).length > 8}
          <button class="link" onclick={() => (showAll = !showAll)}>{showAll ? 'Show fewer libraries' : `Show all ${byKind(k).length} libraries`}</button>
        {/if}
      {/if}
    {/each}

    <h2>Source code</h2>
    <p>
      The complete source of this application is at
      <a href="https://github.com/AlanDrake/romcure" target="_blank" rel="noreferrer">github.com/AlanDrake/romcure</a>.
    </p>
    <p>
      The converter programs shipped with the app (chdman from MAME{#each st.Tools ?? [] as t}{#if t.Name === 'chdman' && t.Version}, version {t.Version}{/if}{/each},
      and dolphin-tool from Dolphin) are licensed under the GNU General Public License, and 7-Zip, which reads RAR files, under the GNU Lesser General Public License.
      You are entitled to their complete corresponding source code.
      It is attached to every
      <a href="https://github.com/AlanDrake/romcure/releases" target="_blank" rel="noreferrer">RomCure release</a>,
      for the exact versions that release ships.
      These are the projects' own unmodified releases, and their source is also published by them: MAME at
      <a href="https://github.com/mamedev/mame/releases" target="_blank" rel="noreferrer">github.com/mamedev/mame</a>,
      Dolphin at <a href="https://github.com/dolphin-emu/dolphin" target="_blank" rel="noreferrer">github.com/dolphin-emu/dolphin</a>,
      7-Zip at <a href="https://github.com/ip7z/7zip" target="_blank" rel="noreferrer">github.com/ip7z/7zip</a>
      (the versions are listed above under Third-party licenses). If you cannot find a source archive,
      write to the
      <a href="https://github.com/AlanDrake/romcure/issues" target="_blank" rel="noreferrer">project's issue tracker</a>
      and it will be provided.
    </p>

    <h2>Troubleshooting</h2>
    <p class="muted">Answers to common questions: the
      <a href="https://github.com/AlanDrake/romcure/blob/master/docs/FAQ.md" target="_blank" rel="noreferrer">FAQ</a>.
      To report a problem, open an issue on
      <a href="https://github.com/AlanDrake/romcure/issues" target="_blank" rel="noreferrer">GitHub</a> with the diagnostics file below.</p>
    <p class="muted">Data folder: <span class="path">{st.DataDir}</span></p>
    <button onclick={exportLogs}>Save diagnostics…</button>
    <span class="muted">A zip with the recent log and settings, with your user name removed. No ROM names or library data.</span>
  </section>
{/if}
