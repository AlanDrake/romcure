<script lang="ts">
  import * as Service from '../../bindings/romcure/cmd/romcure/service';
  import type {Candidate, Suggestion} from '../../bindings/romcure/internal/discover/models';
  import {app, refresh, fail, ok, start, relaunchElevated, keepCopies, message} from '../lib/state.svelte';
  import RetroArchPicker from '../lib/RetroArchPicker.svelte';
  import {num, plural} from '../lib/games';

  const STEPS = ['Welcome', 'Your games', 'Versions', 'Outputs', 'Links', 'Storage', 'Databases', 'Finish'];
  let step = $state(0);
  let error = $state('');
  let busy = $state(false);
  let alerts: HTMLElement | undefined = $state();

  // An error appears at the top of the card; bring it into view if the page is scrolled down.
  $effect(() => {
    if ((error || app.error) && alerts) alerts.scrollIntoView({block: 'center', behavior: 'smooth'});
  });

  // ---- step 6: cache folder
  let cacheDir = $state(app.st?.Settings.cache_dir ?? '');
  async function pickCache() {
    try {
      const dir = await Service.PickFolder('Choose where converted discs are kept (can grow to many GB)');
      if (dir) cacheDir = dir;
    } catch (e) {
      error = String(e);
    }
  }
  async function commitCache(): Promise<boolean> {
    if (!app.st) return false;
    try {
      await Service.SaveSettings({...app.st.Settings, cache_dir: cacheDir});
      await refresh();
      return true;
    } catch (e) {
      error = message(e);
      return false;
    }
  }

  // ---- step 2: where the games are
  type Row = {include: boolean; folder: string; name: string; systemId: string; games: number; reason: string};
  let parent = $state('');
  let rows: Row[] = $state([]);
  let looked = $state(false);
  const added = new Set<string>();

  async function browse() {
    try {
      const dir = await Service.PickFolder('Choose the folder that holds your games');
      if (dir) {
        parent = dir;
        await look();
      }
    } catch (e) {
      error = String(e);
    }
  }
  async function look() {
    if (!parent.trim()) return;
    error = '';
    busy = true;
    try {
      const found: Candidate[] = (await Service.Discover(parent.trim())) ?? [];
      const have = new Set<string>(app.st?.Sources?.map((s) => s.Path) ?? []);
      rows = found.map((c) => ({
        include: !!c.SystemID && !have.has(c.Folder),
        folder: c.Folder, name: c.Name, systemId: c.SystemID, games: c.Games, reason: c.Reason,
      }));
      looked = true;
    } catch (e) {
      error = message(e);
    } finally {
      busy = false;
    }
  }
  const chosen = $derived(rows.filter((r) => r.include));
  const haveSources = $derived((app.st?.Sources?.length ?? 0) > 0);

  async function commitSources(): Promise<boolean> {
    for (const r of chosen) {
      if (!added.has(r.folder) && !r.systemId) {
        error = `Choose which system “${r.name}” holds, or untick it.`;
        return false;
      }
    }
    for (const r of chosen) {
      if (added.has(r.folder)) continue;
      try {
        await Service.AddSource(r.folder, r.systemId);
        added.add(r.folder);
      } catch (e) {
        error = message(e);
        await refresh();
        return false;
      }
    }
    await refresh();
    return true;
  }

  // ---- step 3: versions
  let preset = $state(app.st?.Settings.region_preset ?? 'usa');
  const presetInfo = $derived(app.st?.Presets?.find((p) => p.ID === preset));
  const example: Record<string, string> = {
    usa: 'If a game exists in USA, Europe and Japan versions, you get the USA one.',
    europe: 'If a game exists in USA, Europe and Japan versions, you get the European one.',
    japan: 'If a game exists in USA, Europe and Japan versions, you get the Japanese one.',
    world: 'If a game has a World (region-free) version you get it; otherwise USA, then Europe, then Japan.',
  };
  async function commitPreset(): Promise<boolean> {
    if (!app.st) return false;
    try {
      await Service.SaveSettings({...app.st.Settings, region_preset: preset});
      await refresh();
      return true;
    } catch (e) {
      error = message(e);
      return false;
    }
  }

  // ---- step 4: outputs
  let suggestions: Suggestion[] = $state([]);
  let pick = $state(-2); // -2 = nothing chosen yet, -1 = somewhere else, -3 = not now, >= 0 a suggestion
  let oName = $state('');
  let oKind = $state('retroarch');
  let oFolder = $state('');
  let oStart = $state('all');
  let oConfig = $state('');
  let outputAdded = $state(false);
  const kinds = [
    ['retroarch', 'RetroArch'], ['esde', 'ES-DE'], ['retrodeck', 'RetroDECK'], ['batocera', 'Batocera'],
    ['retrobat', 'RetroBat'], ['raw', 'Plain folders'], ['device', 'SD card / USB (copy)'],
  ];
  async function loadSuggestions() {
    try {
      suggestions = (await Service.OutputSuggestions()) ?? [];
    } catch {
      suggestions = [];
    }
  }
  function choose(i: number) {
    pick = i;
    if (i >= 0) {
      const s = suggestions[i];
      oName = s.Label.startsWith('SD card') ? 'SD card' : s.Label;
      oKind = s.Kind;
      oFolder = s.Path;
    } else if (i === -1) {
      oName = '';
      oFolder = '';
    }
    oStart = oKind === 'device' ? 'none' : 'all';
  }
  async function browseOutput() {
    try {
      const dir = await Service.PickFolder('Choose where to create the output');
      if (dir) oFolder = dir;
    } catch (e) {
      error = String(e);
    }
  }
  async function commitOutput(): Promise<boolean> {
    if (pick === -3 || pick === -2 || outputAdded) return true;
    if (!oName.trim() || !oFolder.trim()) {
      error = 'Give the output a name and a folder, or choose “Not now”.';
      return false;
    }
    try {
      await Service.AddProfile(oName.trim(), oKind, oFolder.trim(), null, oStart === 'none', oKind === 'retroarch' ? oConfig : '');
      outputAdded = true;
      await refresh();
      return true;
    } catch (e) {
      error = message(e);
      return false;
    }
  }

  // ---- navigation
  async function next() {
    error = '';
    busy = true;
    try {
      if (step === 1 && !(await commitSources())) return;
      if (step === 1 && !chosen.length && !haveSources) {
        error = 'Choose at least one folder to continue.';
        return;
      }
      if (step === 2 && !(await commitPreset())) return;
      if (step === 3 && !(await commitOutput())) return;
      if (step === 5 && !(await commitCache())) return;
      step++;
      if (step === 3) await loadSuggestions();
    } finally {
      busy = false;
    }
  }
  function back() {
    error = '';
    step = Math.max(0, step - 1);
  }
  async function finish(runSync: boolean) {
    if (!app.st) return;
    try {
      await Service.SaveSettings({...app.st.Settings, onboarded: true});
      await refresh();
      app.setupOpen = false;
      app.tab = runSync ? 'games' : 'library';
      if (runSync) await start('sync', () => Service.StartSync());
      else ok('Setup finished. Press Sync now on the Library tab when you are ready.');
    } catch (e) {
      fail(e);
    }
  }
  async function updateDatabases() {
    await start('dats', () => Service.StartDATUpdate());
  }
</script>

<div class="setup">
  <div class="card">
    <ol class="dots" aria-label="Setup steps">
      {#each STEPS as s, i}<li class:on={i === step} class:done={i < step}><span>{i + 1}</span>{s}</li>{/each}
    </ol>

    <div bind:this={alerts}>
      {#if app.error}<div class="msg err" role="alert">{app.error}</div>{/if}
      {#if error}<div class="msg err" role="alert">{error}</div>{/if}
    </div>

    {#if step === 0}
      <h2>Welcome to RomCure</h2>
      <p>This keeps one good copy of each game from your big collection and makes it appear in your emulators, without copying files.</p>
      <ul class="points">
        <li>It <strong>reads</strong> your game folders and never changes them.</li>
        <li>It picks <strong>one version</strong> of each game (for example the USA one) and tells you why.</li>
        <li>It creates <strong>outputs</strong>: folders arranged for RetroArch, ES-DE, or an SD card, using links where it can.</li>
      </ul>
      <p class="muted">Setup takes a minute. You can change everything later.</p>
    {:else if step === 1}
      <h2>Where are your games?</h2>
      <p class="muted">Choose the folder that holds them. If it has one folder per system, they are all found at once.</p>
      <div class="form">
        <input class="grow" placeholder="Folder with your games" aria-label="Folder with your games" bind:value={parent} onkeydown={(e) => e.key === 'Enter' && look()} />
        <button onclick={browse}>Browse…</button>
        <button class="primary" onclick={look} disabled={!parent.trim() || busy}>Look here</button>
      </div>
      {#if looked && !rows.length}<p class="empty">No game files were found there.</p>{/if}
      {#if rows.length}
        <table class="found">
          <thead><tr><th></th><th>Folder</th><th>Games</th><th>System</th></tr></thead>
          <tbody>
            {#each rows as r (r.folder)}
              <tr>
                <td><input type="checkbox" bind:checked={r.include} aria-label="Use {r.name}" disabled={added.has(r.folder)} /></td>
                <td><b>{r.name}</b><span class="path">{r.folder}</span></td>
                <td>{num(r.games)}</td>
                <td>
                  <select bind:value={r.systemId} aria-label="System of {r.name}" disabled={added.has(r.folder)}>
                    <option value="">Not sure…</option>
                    {#each app.st?.Systems ?? [] as s}<option value={s.ID}>{s.Name}</option>{/each}
                  </select>
                  {#if r.reason}<span class="muted">guessed from {r.reason}</span>{/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
        <p class="muted">{plural(chosen.length, 'folder')} selected. Untick the ones you do not want. Several folders of one system are fine: they become one library.</p>
      {/if}
      {#if haveSources}<p class="muted">Already set up: {(app.st?.Sources ?? []).map((s) => s.Short).join(', ')}.</p>{/if}
    {:else if step === 2}
      <h2>Which versions do you prefer?</h2>
      <p class="muted">Many games exist in several regions. The app keeps one and shows you which, so you can always pick another.</p>
      <label>
        Preference
        <select bind:value={preset} aria-label="Region preference">
          {#each app.st?.Presets ?? [] as p}<option value={p.ID}>{p.Name}</option>{/each}
        </select>
      </label>
      <p>{presetInfo?.Description}</p>
      <p class="example">{example[preset] ?? ''}</p>
    {:else if step === 3}
      <h2>Where should your games go?</h2>
      <p class="muted">An output is a folder for one program. You can add more later on the Outputs tab.</p>
      <div class="choices" role="radiogroup" aria-label="Output">
        {#each suggestions as s, i}
          <label class="choice"><input type="radio" name="out" checked={pick === i} onchange={() => choose(i)} />
            <span><b>{s.Label}</b><span class="muted">{s.Note}</span><span class="path">{s.Path}</span></span></label>
        {/each}
        <label class="choice"><input type="radio" name="out" checked={pick === -1} onchange={() => choose(-1)} />
          <span><b>Somewhere else</b><span class="muted">Pick the program and the folder yourself.</span></span></label>
        <label class="choice"><input type="radio" name="out" checked={pick === -3} onchange={() => choose(-3)} />
          <span><b>Not now</b><span class="muted">Skip this; add an output later.</span></span></label>
      </div>
      {#if pick >= -1 && pick !== -3 && pick !== -2}
        <div class="form col">
          <input placeholder="Name, e.g. My RetroArch" aria-label="Output name" bind:value={oName} />
          {#if pick === -1}
            <select bind:value={oKind} aria-label="Kind of output" onchange={() => (oStart = oKind === 'device' ? 'none' : 'all')}>
              {#each kinds as [id, label]}<option value={id}>{label}</option>{/each}
            </select>
          {/if}
          <div class="form">
            <input class="grow" placeholder="Output folder" aria-label="Output folder" bind:value={oFolder} />
            <button onclick={browseOutput}>Browse…</button>
          </div>
          {#if oKind === 'retroarch'}<RetroArchPicker bind:value={oConfig} />{/if}
          <label>
            Start with
            <select bind:value={oStart} aria-label="Start with">
              <option value="all">All games</option>
              <option value="none">No games (I'll choose)</option>
            </select>
          </label>
        </div>
      {/if}
    {:else if step === 4}
      <h2>Links</h2>
      {#if app.st?.Links?.SymlinksOK}
        <p>Links work on this computer, so outputs take almost no extra space. Nothing to do here.</p>
      {:else if app.st?.Links?.Offer}
        <p>Creating links needs administrator rights on this PC (or Windows Developer Mode). You can:</p>
        <div class="form">
          <button onclick={() => relaunchElevated()}>Restart as administrator</button>
          <button onclick={() => keepCopies()}>Use copies instead</button>
        </div>
        <p class="muted">Copies use more disk space but work everywhere. Restarting asks Windows for permission once; you would continue setup afterwards.</p>
      {:else}
        <p>Links are not available here, so outputs will use copies. That works, but uses more disk space.</p>
      {/if}
    {:else if step === 5}
      <h2>Where to keep converted games</h2>
      <p class="muted">Discs are converted to smaller files (CHD, RVZ) and kept here, and the originals are never touched. This can grow to many GB, so pick a drive with room.</p>
      <div class="form">
        <button onclick={pickCache}>Choose…</button>
        <span class="path" id="cache-path">{cacheDir || 'Default: ' + (app.st?.CacheRoot ?? '')}</span>
        {#if cacheDir}<button class="quiet" onclick={() => (cacheDir = '')}>Use default</button>{/if}
      </div>
      <p class="muted">You can change this later in Settings. Games already converted stay where they are.</p>
    {:else if step === 6}
      <h2>Game databases (optional)</h2>
      <p>Databases (DAT files) let the app name your files exactly and tell good dumps from bad ones. The app only goes online when you press the button.</p>
      <div class="form">
        <button class="primary" onclick={updateDatabases} disabled={!!app.job || !haveSources}>{app.job === 'dats' ? 'Updating…' : 'Download databases'}</button>
      </div>
      <p class="muted">You can skip this and do it later on the Library tab.</p>
    {:else}
      <h2>Ready</h2>
      <p>The first sync reads your folders, picks a version of each game, and fills your output. It can take a while for big collections; you can keep using the app, and Cancel is always on the top bar.</p>
      <ul class="points">
        <li>{plural(app.st?.Sources?.length ?? 0, 'game folder')} added</li>
        <li>{(app.st?.Profiles?.length ?? 0) ? plural(app.st?.Profiles?.length ?? 0, 'output') + ' added' : 'No output yet (add one later)'}</li>
      </ul>
    {/if}

    <div class="nav">
      <button class="quiet" onclick={() => finish(false)}>Skip setup</button>
      <span class="grow"></span>
      {#if step > 0}<button onclick={back} disabled={busy}>Back</button>{/if}
      {#if step < STEPS.length - 1}
        <button class="primary" onclick={next} disabled={busy}>Next</button>
      {:else}
        <button class="primary" onclick={() => finish(true)} disabled={!!app.job}>Start first sync</button>
      {/if}
    </div>
  </div>
</div>
