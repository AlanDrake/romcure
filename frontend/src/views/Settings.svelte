<script lang="ts">
  import * as Service from '../../bindings/romcure/cmd/romcure/service';
  import {app, refresh, fail, ok} from '../lib/state.svelte';

  async function saveSettings() {
    if (!app.st) return;
    try {
      await Service.SaveSettings(app.st.Settings);
      ok('Settings saved.');
      await refresh();
    } catch (e) {
      fail(e);
    }
  }
  async function pickCache() {
    if (!app.st) return;
    try {
      const dir = await Service.PickFolder('Choose where converted discs are kept (can grow to many GB)');
      if (dir) app.st.Settings.cache_dir = dir;
    } catch (e) {
      fail(e);
    }
  }
  async function pickScratch() {
    if (!app.st) return;
    try {
      const dir = await Service.PickFolder('Choose a scratch folder (needs several GB free)');
      if (dir) app.st.Settings.scratch_dir = dir;
    } catch (e) {
      fail(e);
    }
  }
</script>

{#if app.st}
  {@const st = app.st}
  <section>
    <h2>Settings</h2>
    <label>
      Region preference
      <select bind:value={st.Settings.region_preset}>
        {#each st.Presets ?? [] as p}<option value={p.ID}>{p.Name}</option>{/each}
      </select>
      <span class="muted">{st.Presets?.find((p) => p.ID === st.Settings.region_preset)?.Description}</span>
    </label>
    <label>
      Conversions at once
      <input type="number" min="1" max="8" bind:value={st.Settings.parallelism} />
      <span class="muted">Keep at 1 for DVD-size games unless you have lots of free disk space.</span>
    </label>
    <label>
      CD codecs (CHD)
      <input type="text" placeholder="cdlz,cdzl,cdfl" aria-label="CD codecs" bind:value={st.Settings.chd_codecs_cd} />
      <span class="muted">For PlayStation, Saturn, Sega CD and other CD games. Leave empty for the default, which every RetroArch core reads. Up to four of: cdlz, cdzl, cdfl, cdzs, lzma, zlib, huff, flac, zstd, none. Changing it converts discs again the next time they are needed.</span>
    </label>
    <label>
      DVD codecs (CHD)
      <input type="text" placeholder="lzma,zlib,huff,flac" aria-label="DVD codecs" bind:value={st.Settings.chd_codecs_dvd} />
      <span class="muted">For PlayStation 2 and PSP discs. Leave empty for the default.</span>
    </label>
    <label>
      Cache folder
      <div class="form">
        <button onclick={pickCache}>Choose…</button>
        <span class="path">{st.Settings.cache_dir || 'Default: ' + st.CacheRoot}</span>
        {#if st.Settings.cache_dir}<button class="quiet" onclick={() => (st.Settings.cache_dir = '')}>Use default</button>{/if}
      </div>
      <span class="muted">Where converted discs (CHD, RVZ) and extracted games are kept; it can grow to many GB. Files already converted stay where they are and keep being used.</span>
    </label>
    <label>
      Scratch folder
      <div class="form">
        <button onclick={pickScratch}>Choose…</button>
        <span class="path">{st.Settings.scratch_dir || 'Default: ' + st.ScratchDir}</span>
        {#if st.Settings.scratch_dir}<button class="quiet" onclick={() => (st.Settings.scratch_dir = '')}>Use default</button>{/if}
      </div>
      <span class="muted">Used while converting discs; Wii/GameCube images need about 10 GB free.</span>
    </label>
    <label class="check">
      <input type="checkbox" checked={!st.Settings.no_default_cores} onchange={(e) => (st.Settings.no_default_cores = !e.currentTarget.checked)} />
      Set a default core on new RetroArch playlists
      <span class="muted">When a playlist has no default core, RomCure picks an installed one so games open without asking: one that can open the files and has its BIOS, preferring the core you already play those games with, then the recommended one (for example mGBA for Game Boy Advance). The sync report says which and why. A core already set in a playlist, or on a game, is never changed.</span>
    </label>
    <label class="check">
      <input type="checkbox" bind:checked={st.Settings.allow_hardlink} />
      Allow hard links (advanced)
      <span class="muted">Only for outputs on the same drive as your games. Editing such a file edits the original.</span>
    </label>
    <label>
      When Windows needs administrator rights to make links
      <select bind:value={st.Settings.elevation}>
        <option value="ask">Ask me</option>
        <option value="never">Never ask; use another method</option>
      </select>
    </label>
    <button class="primary" onclick={saveSettings}>Save settings</button>
  </section>
{/if}
