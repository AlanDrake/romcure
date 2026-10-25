<script lang="ts">
  import * as Service from '../../bindings/romcure/cmd/romcure/service';
  import {app, refresh, fail, ok, start} from '../lib/state.svelte';
  import {baseName} from '../lib/games';

  let newSystem = $state('');
  let newPath = $state('');

  async function browseSource() {
    try {
      const dir = await Service.PickFolder('Choose the folder with your games');
      if (dir) newPath = dir;
    } catch (e) {
      fail(e);
    }
  }
  async function addSource() {
    if (!newSystem) return fail('Choose the system this folder holds first.');
    if (!newPath.trim()) return fail('Choose or type the folder first.');
    try {
      await Service.AddSource(newPath.trim(), newSystem);
      ok('Added ' + newPath.trim());
      newSystem = '';
      newPath = '';
      await refresh();
    } catch (e) {
      fail(e);
    }
  }
  async function removeSource(id: number) {
    try {
      await Service.RemoveSource(id);
      await refresh();
    } catch (e) {
      fail(e);
    }
  }
  // Open the Games screen on a system (and a filter).
  function show(f: {system: string; unverified?: boolean}) {
    app.gamesFocus = f;
    app.tab = 'games';
  }
  const startSync = () => start('sync', () => Service.StartSync());
  const startDATs = () => start('dats', () => Service.StartDATUpdate());
  const startCHD = () => start('chd', () => Service.StartCHDCheck());
  // ---- import a DAT by hand (No-Intro for cartridge systems)
  let datPath = $state('');
  let datSystem = $state('');
  async function browseDAT() {
    try {
      const p = await Service.PickDATFile();
      if (p) datPath = p;
    } catch (e) {
      fail(e);
    }
  }
  async function importDAT() {
    if (!datPath.trim()) return fail('Choose or type the DAT file first.');
    try {
      const r = await Service.ImportDAT(datPath.trim(), datSystem);
      const sys = st().Systems?.find((s) => s.ID === r.SystemID)?.Name ?? r.SystemID;
      ok(`Imported “${r.Name}” for ${sys}: ${r.Games.toLocaleString('en-US')} ${r.Games === 1 ? 'game' : 'games'}${r.ParentClone ? ', with parent/clone groups' : ''}. Press Sync now to use it.`);
      datPath = '';
    } catch (e) {
      fail(e);
    }
  }
  const st = () => app.st!;
  const chdmanFound = () => app.st?.Tools?.find((t) => t.Name === 'chdman')?.Found ?? false;
</script>

{#if app.st}
  {@const st = app.st}
  <section>
    <h2>Your ROM folders</h2>
    <p class="muted">
      Point the app at folders that hold your games and say which system each holds. A system can have several
      folders; they become one library. Your files are only ever read, never changed.
    </p>
    {#if st.Sources?.length}
      <ul class="rows">
        {#each st.Sources as s}
          <li>
            <div><strong>{s.SystemName}</strong><span class="path">{s.Path}</span></div>
            <button class="quiet" onclick={() => removeSource(s.ID)}>Remove</button>
          </li>
        {/each}
      </ul>
    {:else}
      <p class="empty">No folders yet. Choose a system, then pick its folder.</p>
    {/if}
    <div class="form">
      <select bind:value={newSystem} aria-label="System">
        <option value="">System…</option>
        {#each st.Systems ?? [] as sys}<option value={sys.ID}>{sys.Name}</option>{/each}
      </select>
      <input class="grow" placeholder="Folder path" aria-label="Folder path" bind:value={newPath} />
      <button onclick={browseSource}>Browse…</button>
      <button class="primary" disabled={!newSystem || !newPath.trim()} onclick={addSource}>Add folder</button>
    </div>
  </section>

  <section>
    <h2>Sync</h2>
    <p class="muted">Identifies your games, picks one version of each, and updates every output.</p>
    <div class="form">
      <button class="primary" disabled={!!app.job || !st.Sources?.length} onclick={startSync}>
        {app.job === 'sync' ? 'Syncing…' : 'Sync now'}
      </button>
    </div>

    {#if st.LastReport?.Unreadable?.length}
      {@const bad = st.LastReport.Unreadable}
      <details class="unreadable" id="unreadable" open>
        <summary>
          <b>{bad.length} {bad.length === 1 ? 'file' : 'files'} could not be read</b> and {bad.length === 1 ? 'was' : 'were'} skipped.
          The rest of your library was synced. These are probably damaged; check or replace them.
        </summary>
        <ul class="rows">
          {#each bad.slice(0, 200) as f}
            <li><div><strong>{baseName(f.Path)}</strong><span class="path">{f.Path}</span><span class="detail bad">{f.Reason}</span></div></li>
          {/each}
        </ul>
        {#if bad.length > 200}<p class="muted">Showing the first 200 of {bad.length}.</p>{/if}
      </details>
    {/if}

    {#if st.LastReport?.Systems?.length}
      <table>
        <thead><tr><th>System</th><th>Kept</th><th>Skipped duplicates</th><th>Unidentified</th></tr></thead>
        <tbody>
          {#each st.LastReport.Systems as r}
            <tr>
              <td><button class="link" onclick={() => show({system: r.SystemID})}>{r.SystemID}</button></td>
              <td>{r.Selected}</td>
              <td>{r.Excluded}</td>
              <td>{#if r.Unmatched}<button class="link" onclick={() => show({system: r.SystemID, unverified: true})}>{r.Unmatched}</button>{:else}0{/if}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>

  <section>
    <h2>Game databases</h2>
    <p class="muted">
      DAT files list every known good copy of each game, so the app can name your files exactly and tell real dumps
      from bad ones. Disc systems use Redump (with LibCrypt SBI files for PlayStation); home computers use TOSEC;
      cartridge systems use No-Intro, from the copy libretro-database keeps. PlayStation and PlayStation 2 also
      get a serial list (libretro-database) that names discs no DAT recognizes.
      The app only goes online when you press this button.
    </p>
    <div class="form">
      <button disabled={!!app.job || !st.Sources?.length} onclick={startDATs}>
        {app.job === 'dats' ? 'Updating…' : 'Update DATs'}
      </button>
      <button disabled={!!app.job || !st.Sources?.length || !chdmanFound()} onclick={startCHD}
        title={chdmanFound() ? '' : 'chdman was not found'}>
        {app.job === 'chd' ? 'Checking…' : 'Check CHD files'}
      </button>
    </div>
    <p class="muted">
      “Check CHD files” reads every CHD in your disc folders once so they can be matched to Redump; it takes about
      half a minute per disc.
    </p>
    <h3>Import a DAT file</h3>
    <p class="muted">
      libretro's No-Intro copies can trail No-Intro by a few weeks. For the newest data, or a parent/clone DAT
      (which groups a game's versions most exactly), download it from datomatic.no-intro.org (a .zip is fine) and
      import it here: it replaces the downloaded one, and “Update DATs” then leaves that system alone. A newer
      import for the same system replaces the older one.
    </p>
    <div class="form">
      <select bind:value={datSystem} aria-label="DAT system">
        <option value="">System: from the file</option>
        {#each st.Systems ?? [] as sys}<option value={sys.ID}>{sys.Name}</option>{/each}
      </select>
      <input class="grow" placeholder="DAT file (.dat, .xml or .zip)" aria-label="DAT file" bind:value={datPath} />
      <button onclick={browseDAT}>Browse…</button>
      <button disabled={!!app.job || !datPath.trim()} onclick={importDAT}>Import DAT</button>
    </div>
    {#if app.datResults.length}
      <ul class="rows">
        {#each app.datResults as r}
          <li>
            <div>
              <strong>{r.SystemID}</strong>
              {#if r.Source}<span class="tag">{r.Source}</span>{/if}
              {#if r.Error}<span class="detail bad">{r.Error}</span>
              {:else}<span class="detail">{r.Message}</span>{/if}
            </div>
          </li>
        {/each}
      </ul>
    {/if}
    {#if app.chdResults.length}
      <ul class="rows">
        {#each app.chdResults as r}
          <li>
            <div>
              <strong>{r.SystemID}</strong><span class="detail">{r.Checked} CHD files checked</span>
              {#each r.Errors ?? [] as e}<span class="detail bad">{e}</span>{/each}
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}
