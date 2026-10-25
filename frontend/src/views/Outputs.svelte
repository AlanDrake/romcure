<script lang="ts">
  import {untrack} from 'svelte';
  import * as Service from '../../bindings/romcure/cmd/romcure/service';
  import type {OutputProblem, OutputSummary} from '../../bindings/romcure/internal/app/models';
  import {app, refresh, fail, ok, start} from '../lib/state.svelte';
  import {num, plural, size} from '../lib/games';

  let pName = $state('');
  let pKind = $state('retroarch');
  let pFolder = $state('');
  let pStart = $state('all');

  const kinds: {id: string; label: string; hint: string}[] = [
    {id: 'retroarch', label: 'RetroArch', hint: 'Links + playlists (.lpl) for RetroArch'},
    {id: 'esde', label: 'ES-DE', hint: 'Folder per system for EmulationStation-DE'},
    {id: 'retrodeck', label: 'RetroDECK', hint: 'ES-DE layout inside RetroDECK'},
    {id: 'batocera', label: 'Batocera', hint: 'Batocera ROM folder names'},
    {id: 'retrobat', label: 'RetroBat', hint: 'RetroBat ROM folder names'},
    {id: 'raw', label: 'Plain folders', hint: 'One clean folder per system, nothing else'},
    {id: 'device', label: 'SD card / USB (copy)', hint: 'Real copies for FAT32/exFAT devices'},
  ];
  const kindLabel = (id: string) => kinds.find((k) => k.id === id)?.label ?? id;
  const resultsFor = (name: string) => app.st?.LastReport?.Profiles?.filter((p) => p.Profile === name) ?? [];

  // An SD card can't hold a whole library, so it starts empty; other outputs start with everything.
  $effect(() => {
    pStart = pKind === 'device' ? 'none' : 'all';
  });

  // Each output's size and waiting changes.
  let sums: Record<number, OutputSummary> = $state({});
  async function loadSums() {
    try {
      const list = (await Service.Outputs()) ?? [];
      sums = Object.fromEntries(list.map((s) => [s.ID, s]));
    } catch (e) {
      fail(e);
    }
  }
  $effect(() => {
    app.st?.Profiles;
    app.syncDone;
    untrack(loadSums);
  });

  async function pickOutput() {
    try {
      const dir = await Service.PickFolder('Choose where to create the output');
      if (dir) pFolder = dir;
    } catch (e) {
      fail(e);
    }
  }
  async function addProfile() {
    try {
      await Service.AddProfile(pName, pKind, pFolder, null, pStart === 'none');
      ok('Added output "' + pName + '"');
      pName = '';
      pFolder = '';
      await refresh();
    } catch (e) {
      fail(e);
    }
  }
  // Which output is asking "also delete its links?" (inline, no dialog)
  let removing: number | null = $state(null);
  async function removeProfile(id: number, deleteLinks: boolean) {
    try {
      await Service.RemoveProfile(id, deleteLinks);
      removing = null;
      await refresh();
    } catch (e) {
      fail(e);
    }
  }
  // Edit name / folder (inline)
  let editing: number | null = $state(null);
  let eName = $state('');
  let eFolder = $state('');
  let eDelete = $state(true);
  function startEdit(p: {ID: number; Name: string; OutputRoot: string}) {
    editing = p.ID;
    eName = p.Name;
    eFolder = p.OutputRoot;
    eDelete = true;
  }
  async function browseEdit() {
    try {
      const dir = await Service.PickFolder('Choose where this output is created');
      if (dir) eFolder = dir;
    } catch (e) {
      fail(e);
    }
  }
  async function saveEdit() {
    if (editing === null) return;
    try {
      await Service.EditOutput(editing, eName, eFolder, eDelete);
      ok('Output saved.');
      editing = null;
      await refresh();
      await loadSums();
    } catch (e) {
      fail(e);
    }
  }

  // Check the files an output made, and repair them
  let problems: Record<number, OutputProblem[]> = $state({});
  async function check(id: number) {
    try {
      problems[id] = (await Service.OutputProblems(id)) ?? [];
    } catch (e) {
      fail(e);
    }
  }
  async function repair(id: number) {
    await start('apply', async () => {
      await Service.RepairOutput(id);
      delete problems[id];
    });
  }

  function chooseGames(id: number) {
    try {
      localStorage.setItem('romcure.games.output', String(id));
    } catch {}
    app.gamesFocus = {outputId: id};
    app.tab = 'games';
  }
  const apply = (id: number) => start('apply', () => Service.StartApply([id]));

  function waiting(s: OutputSummary | undefined): string {
    const p = s?.Pending;
    if (!p) return '';
    const parts: string[] = [];
    if (p.Add) parts.push(`+${num(p.Add)} to add`);
    if (p.Remove) parts.push(`−${num(p.Remove)} to remove`);
    if (p.Replace) parts.push(plural(p.Replace, 'version change'));
    return parts.join(' · ');
  }
</script>

{#if app.st}
  {@const st = app.st}
  <section>
    <h2>Outputs</h2>
    <p class="muted">
      Each output is a folder arranged for one program. They share your library without copying it, and each one has its
      own choice of games.
    </p>
    {#if st.Profiles?.length}
      <div class="cards">
        {#each st.Profiles as p (p.ID)}
          {@const s = sums[p.ID]}
          {@const dev = s?.Pending?.Device}
          <div class="card out">
            <div class="top">
              <div>
                <strong>{p.Name}</strong> <span class="tag">{kindLabel(p.Kind)}</span>
                <span class="path">{p.OutputRoot}</span>
              </div>
              {#if s}<div class="count"><b>{num(s.Included)}</b> of {num(s.Total)} games</div>{/if}
            </div>

            <p class="state">
              {#if !s}<span class="muted">…</span>
              {:else if s.Pending.Any}<b>{waiting(s)}</b> <span class="muted">not applied yet</span>
              {:else}<span class="good">✓</span> Up to date{/if}
              {#if dev}
                <span class="muted"> · needs {size(s?.Pending.Bytes ?? 0, s?.Pending.Estimated)} · {size(dev.FreeBytes)} free · {dev.FSType}</span>
              {/if}
            </p>
            {#each dev?.Blockers ?? [] as b}<p class="state bad">{b}</p>{/each}
            {#if dev?.Oversize?.length}<p class="state bad">{plural(dev.Oversize.length, 'game')} over the {size(dev.MaxFileBytes)} file limit will be skipped.</p>{/if}

            {#each resultsFor(p.Name) as r}
              <span class="result">
                {r.SystemID}: {r.Linked} games
                {#if r.Broken || r.Stale}<em class="bad">{r.Broken} broken, {r.Stale} stale</em>{/if}
                {#if r.Errors?.length}<em class="bad">{r.Errors.length} problems</em>{/if}
                {#if r.Blockers?.length}<em class="bad">{r.Blockers.join('; ')}</em>{/if}
              </span>
              {#each (r.Errors ?? []).slice(0, 3) as e}<span class="detail">{e}</span>{/each}
              {#each r.Notes ?? [] as n}<span class="detail">{n}</span>{/each}
            {/each}

            {#if editing === p.ID}
              <div class="form col edit">
                <input aria-label="Output name" bind:value={eName} />
                <div class="form">
                  <input class="grow" aria-label="Output folder" bind:value={eFolder} />
                  <button onclick={browseEdit}>Browse…</button>
                </div>
                {#if eFolder.trim() !== p.OutputRoot}
                  <label class="check">
                    <input type="checkbox" bind:checked={eDelete} />
                    Delete the links and copies this output made in the old folder (your games are not touched)
                  </label>
                  <span class="muted">The new folder is filled the next time you press Apply.</span>
                {/if}
                <div class="form">
                  <button class="primary" onclick={saveEdit}>Save</button>
                  <button class="quiet" onclick={() => (editing = null)}>Cancel</button>
                </div>
              </div>
            {/if}
            {#if problems[p.ID]}
              {@const list = problems[p.ID]}
              <div class="problems">
                {#if list.length === 0}
                  <p class="state"><span class="good">✓</span> Every file is where it should be.</p>
                {:else}
                  <p class="state bad">{list.length} {list.length === 1 ? 'file is' : 'files are'} missing or out of date:</p>
                  <ul class="rows">
                    {#each list.slice(0, 50) as f}
                      <li><div><strong>{f.Name}</strong> <span class="tag">{f.Status}</span><span class="detail">{f.Reason}</span></div></li>
                    {/each}
                  </ul>
                  {#if list.length > 50}<p class="muted">Showing the first 50.</p>{/if}
                  <button class="primary" disabled={!!app.job} onclick={() => repair(p.ID)}>Repair</button>
                  <span class="muted">Removes just these files and creates them again.</span>
                {/if}
              </div>
            {/if}
            <div class="form">
              <button onclick={() => chooseGames(p.ID)}>Choose games…</button>
              <button class="primary" disabled={!!app.job || !s?.Pending.Any || !!dev?.Blockers?.length} onclick={() => apply(p.ID)}>Apply</button>
              <button onclick={() => check(p.ID)} title="Look for links and copies that are missing or out of date">Check files</button>
              <button class="quiet" onclick={() => startEdit(p)}>Edit</button>
              <span class="grow"></span>
              {#if removing === p.ID}
                <span class="muted">Also delete the links this output made? Your games are not touched.</span>
                <button onclick={() => removeProfile(p.ID, true)}>Delete links</button>
                <button onclick={() => removeProfile(p.ID, false)}>Keep files</button>
                <button class="quiet" onclick={() => (removing = null)}>Cancel</button>
              {:else}
                <button class="quiet" onclick={() => (removing = p.ID)}>Remove</button>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {:else}
      <p class="empty">No outputs yet.</p>
    {/if}

    <h3>Add an output</h3>
    <div class="form col">
      <input placeholder="Name, e.g. My RetroArch" bind:value={pName} />
      <select bind:value={pKind} aria-label="Kind of output">
        {#each kinds as k}<option value={k.id}>{k.label}</option>{/each}
      </select>
      <span class="muted">{kinds.find((k) => k.id === pKind)?.hint}</span>
      <div class="form">
        <input class="grow" placeholder="Output folder path" aria-label="Output folder path" bind:value={pFolder} />
        <button onclick={pickOutput}>Browse…</button>
      </div>
      <label class="start">
        Start with
        <select bind:value={pStart} aria-label="Start with">
          <option value="all">All games</option>
          <option value="none">No games (I'll choose)</option>
        </select>
        <span class="muted">{pStart === 'all' ? 'New games are added automatically; untick what you do not want.' : 'Only the games you tick are used. Good for an SD card.'}</span>
      </label>
      <button class="primary" disabled={!pName || !pFolder} onclick={addProfile}>Add output</button>
    </div>
  </section>
{/if}
