<script lang="ts">
  import {onMount, untrack} from 'svelte';
  import * as Service from '../../bindings/romcure/cmd/romcure/service';
  import type {Filter, GameRow} from '../../bindings/romcure/internal/catalog/models';
  import type {GameDetail, Pending, SystemCounts, Target} from '../../bindings/romcure/internal/app/models';
  import {app, fail, start} from '../lib/state.svelte';
  import {confidence, highlight, num, plural, size} from '../lib/games';
  import VirtualList from '../lib/VirtualList.svelte';
  import SystemRail from './games/SystemRail.svelte';
  import GameDrawer from './games/GameDrawer.svelte';
  import ChangesBar from './games/ChangesBar.svelte';

  const PAGE = 200;
  const STORE_KEY = 'romcure.games.output';

  // ---- outputs
  const outputs = $derived(app.st?.Profiles ?? []);
  // Another tab can ask to open this screen on an output / system / filter.
  const focus = app.gamesFocus;
  app.gamesFocus = null;
  let outId = $state(focus?.outputId ?? 0);
  const output = $derived(outputs.find((o) => o.ID === outId));
  $effect(() => {
    if (!outputs.length) {
      outId = 0;
      return;
    }
    if (outputs.some((o) => o.ID === outId)) return;
    let saved = 0;
    try {
      saved = Number(localStorage.getItem(STORE_KEY)) || 0;
    } catch {}
    outId = outputs.some((o) => o.ID === saved) ? saved : outputs[0].ID;
  });
  function chooseOutput(id: number) {
    outId = id;
    try {
      localStorage.setItem(STORE_KEY, String(id));
    } catch {}
  }

  const info = $derived.by(() => {
    const m: Record<string, {short: string; name: string}> = {};
    for (const s of app.st?.Systems ?? []) m[s.ID] = {short: s.Short, name: s.Name};
    for (const s of app.st?.Sources ?? []) m[s.SystemID] ??= {short: s.Short, name: s.SystemName};
    return m;
  });
  const short = (id: string) => info[id]?.short ?? id;

  // ---- view state
  let view = $state({query: '', show: 'all', several: false, choices: false, unverified: !!focus?.unverified, sort: 'name', desc: false});
  let sysFilter: string[] = $state(focus?.system ? [focus.system] : []);
  let applied = $state(''); // the search text after the typing pause
  const anyFilter = $derived(view.show !== 'all' || view.several || view.choices || view.unverified || !!applied);

  $effect(() => {
    const q = view.query.trim();
    const t = setTimeout(() => (applied = q), 120);
    return () => clearTimeout(t);
  });

  const filter = (): Filter => ({
    OutputID: outId,
    Systems: [...sysFilter],
    Query: applied,
    Show: view.show,
    Several: view.several,
    Choices: view.choices,
    Unverified: view.unverified,
    Sort: view.sort,
    Desc: view.desc,
  });
  const target = (t: Partial<Target>): Target => ({All: false, SystemID: '', Keys: null, Filter: null, ...t}) as Target;

  // ---- the list
  let pages: Record<number, GameRow[]> = $state({});
  let total = $state(0);
  let includedN = $state(0);
  let other = $state(0);
  let loaded = $state(false);
  let gen = 0;
  const loading = new Set<number>();
  let range = {start: 0, end: 0};
  let list: VirtualList | undefined = $state();
  let active = $state(-1);
  let anchor = -1;

  const rowAt = (i: number): GameRow | undefined => pages[Math.floor(i / PAGE)]?.[i % PAGE];

  async function reload(resetScroll: boolean) {
    const g = ++gen;
    try {
      const p = await Service.Games(filter(), 0, PAGE);
      if (g !== gen) return;
      total = p.Total;
      includedN = p.Included;
      other = p.OtherSystems;
      loading.clear();
      pages = {0: p.Rows ?? []};
      loaded = true;
      if (resetScroll) {
        list?.scrollToTop();
        active = -1;
        anchor = -1;
      }
    } catch (e) {
      fail(e);
    }
    ensure(range.start, range.end);
  }
  function ensure(s: number, e: number) {
    if (e <= s) return;
    for (let p = Math.floor(s / PAGE); p <= Math.floor((e - 1) / PAGE); p++) {
      if (p in pages || loading.has(p)) continue;
      loading.add(p);
      const g = gen;
      Service.Games(filter(), p * PAGE, PAGE)
        .then((pg) => {
          if (g === gen) pages[p] = pg.Rows ?? [];
        })
        .catch(fail)
        .finally(() => loading.delete(p));
    }
  }
  function onrange(s: number, e: number) {
    range = {start: s, end: e};
    ensure(s, e);
  }
  // After a tick the rows on screen are patched in place; only the totals are refreshed.
  async function refreshMeta() {
    try {
      const p = await Service.Games(filter(), 0, 1);
      includedN = p.Included;
      other = p.OtherSystems;
      if (view.show === 'all') total = p.Total;
    } catch (e) {
      fail(e);
    }
  }
  function patchRows(keys: string[], included: boolean) {
    const set = new Set(keys);
    for (const p of Object.values(pages)) for (const r of p) if (set.has(r.Key)) ((r.Included = included), (r.Explicit = true));
  }

  // ---- rail, pending, detail
  let counts: SystemCounts | null = $state(null);
  let pending: Pending | null = $state(null);
  let others: string[] = $state([]);
  let openKey: string | null = $state(null);
  let detail: GameDetail | null = $state(null);

  // ---- the systems rail is resizable, width remembered across visits
  const RAIL_KEY = 'romcure.games.railWidth';
  const RAIL_MIN = 160;
  const RAIL_MAX = 420;
  let railW = $state(216);
  onMount(() => {
    try {
      const saved = Number(localStorage.getItem(RAIL_KEY));
      if (saved >= RAIL_MIN && saved <= RAIL_MAX) railW = saved;
    } catch {}
  });
  function saveRailW() {
    try {
      localStorage.setItem(RAIL_KEY, String(railW));
    } catch {}
  }
  function startRailResize(e: PointerEvent) {
    if (e.button !== 0) return;
    e.preventDefault();
    const startX = e.clientX;
    const startW = railW;
    const handle = e.currentTarget as HTMLElement;
    handle.setPointerCapture(e.pointerId);
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
    const onMove = (ev: PointerEvent) => (railW = Math.min(RAIL_MAX, Math.max(RAIL_MIN, startW + (ev.clientX - startX))));
    const onUp = () => {
      handle.removeEventListener('pointermove', onMove);
      handle.removeEventListener('pointerup', onUp);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      saveRailW();
    };
    handle.addEventListener('pointermove', onMove);
    handle.addEventListener('pointerup', onUp);
  }
  function railResizeKey(e: KeyboardEvent) {
    const step = e.shiftKey ? 40 : 12;
    if (e.key === 'ArrowLeft') railW = Math.max(RAIL_MIN, railW - step);
    else if (e.key === 'ArrowRight') railW = Math.min(RAIL_MAX, railW + step);
    else return;
    e.preventDefault();
    saveRailW();
  }

  async function refreshCounts() {
    try {
      counts = await Service.SystemCounts(outId);
    } catch (e) {
      fail(e);
    }
  }
  async function refreshPending() {
    if (!outId) {
      pending = null;
      others = [];
      return;
    }
    try {
      // One call covers this output and the others waiting for an update.
      const all = (await Service.Outputs()) ?? [];
      pending = all.find((o) => o.ID === outId)?.Pending ?? null;
      const waiting = new Set(all.filter((o) => o.ID !== outId && o.Pending.Any).map((o) => o.ID));
      others = outputs.filter((o) => waiting.has(o.ID)).map((o) => o.Name);
    } catch (e) {
      fail(e);
    }
  }
  async function refreshDetail() {
    if (!openKey) return;
    try {
      detail = await Service.Game(openKey);
    } catch {
      openKey = detail = null; // the game left the library
    }
  }
  async function refreshAll() {
    await Promise.all([reload(false), refreshCounts(), refreshPending(), refreshDetail()]);
  }

  // Reload when the question changes.
  $effect(() => {
    outId;
    applied;
    sysFilter.length;
    sysFilter.join();
    view.show;
    view.several;
    view.choices;
    view.unverified;
    view.sort;
    view.desc;
    if (!app.st) return;
    untrack(() => reload(true));
  });
  $effect(() => {
    outId;
    if (!app.st) return;
    untrack(() => {
      refreshCounts();
      refreshPending();
      refreshDetail();
    });
  });
  // A sync or an apply finished.
  let seenDone = app.syncDone;
  $effect(() => {
    if (app.syncDone === seenDone) return;
    seenDone = app.syncDone;
    untrack(refreshAll);
  });

  // ---- toast
  let toast: {msg: string; token: string; action?: {label: string; run: () => void}} | null = $state(null);
  let toastTimer: ReturnType<typeof setTimeout>;
  function showToast(msg: string, token = '', action?: {label: string; run: () => void}) {
    toast = {msg, token, action};
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => (toast = null), token || action ? 8000 : 4500);
  }
  onMount(() => () => clearTimeout(toastTimer));

  // ---- actions
  async function tick(t: Partial<Target>, included: boolean, patch?: string[]) {
    if (!outId) return;
    try {
      const ch = await Service.SetIncluded(outId, target(t), included);
      if (ch.Affected > 1 || !patch) showToast(ch.Message, ch.Token);
      if (patch) patchRows(patch, included);
      else await reload(false);
      await Promise.all([refreshCounts(), refreshPending(), refreshMeta(), refreshDetail()]);
    } catch (e) {
      fail(e);
    }
  }
  async function onRowTick(i: number, shift: boolean) {
    const r = rowAt(i);
    if (!r || !outId) return;
    let keys = [r.Key];
    if (shift && anchor >= 0 && anchor !== i) {
      const lo = Math.min(anchor, i);
      const hi = Math.max(anchor, i);
      try {
        keys = ((await Service.Games(filter(), lo, hi - lo + 1)).Rows ?? []).map((x) => x.Key);
      } catch (e) {
        return fail(e);
      }
    }
    anchor = i;
    active = i;
    await tick({Keys: keys}, !r.Included, keys);
  }
  const tickShown = () => tick({Filter: filter()}, includedN !== total);
  const tickSystem = (id: string | null, included: boolean) => tick(id ? {SystemID: id} : {All: true}, included);

  function filterSystem(id: string | null, multi: boolean) {
    if (id === null) sysFilter = [];
    else if (multi) sysFilter = sysFilter.includes(id) ? sysFilter.filter((s) => s !== id) : [...sysFilter, id];
    else sysFilter = [id];
  }
  function clearFilters() {
    Object.assign(view, {query: '', show: 'all', several: false, choices: false, unverified: false});
    applied = '';
  }

  async function open(key: string) {
    openKey = key;
    await refreshDetail();
  }
  async function drawerTick(oid: number, included: boolean) {
    if (!openKey) return;
    try {
      const ch = await Service.SetIncluded(oid, target({Keys: [openKey]}), included);
      showToast(ch.Message, ch.Token);
      if (oid === outId) patchRows([openKey], included);
      await Promise.all([refreshDetail(), refreshPending(), oid === outId ? refreshCounts() : Promise.resolve(), refreshMeta()]);
    } catch (e) {
      fail(e);
    }
  }
  async function follow() {
    if (!openKey) return;
    try {
      const ch = await Service.FollowSystem(outId, [openKey]);
      showToast(ch.Message, ch.Token);
      await refreshAll();
    } catch (e) {
      fail(e);
    }
  }
  async function version(gameKey: string | null) {
    if (!openKey) return;
    try {
      const ch = gameKey ? await Service.SetVersion(openKey, gameKey) : await Service.AutoVersion(openKey);
      showToast(ch.Message, ch.Token);
      await refreshAll();
    } catch (e) {
      fail(e);
    }
  }
  async function undo() {
    try {
      const label = await Service.Undo('');
      showToast('Undone: ' + label);
      await refreshAll();
    } catch (e) {
      fail(e);
    }
  }
  const apply = () => start('apply', () => Service.StartApply([outId]));
  const applyAll = () => start('apply', () => Service.StartApply([]));

  // ---- keyboard
  function onkey(e: KeyboardEvent) {
    if (!total) return;
    const page = 10;
    let i = active;
    switch (e.key) {
      case 'ArrowDown': i = Math.min(total - 1, i + 1); break;
      case 'ArrowUp': i = Math.max(0, i - 1); break;
      case 'PageDown': i = Math.min(total - 1, i + page); break;
      case 'PageUp': i = Math.max(0, i - page); break;
      case 'Home': i = 0; break;
      case 'End': i = total - 1; break;
      case ' ': e.preventDefault(); if (i >= 0) onRowTick(i, e.shiftKey); return;
      case 'Enter': { const r = rowAt(i); if (r) open(r.Key); return; }
      default: return;
    }
    e.preventDefault();
    active = i;
    list?.scrollToIndex(i);
    const r = rowAt(i);
    if (openKey && r) open(r.Key);
  }
  let searchEl: HTMLInputElement | undefined = $state();
  function onWindowKey(e: KeyboardEvent) {
    const t = e.target as HTMLElement;
    const typing = t?.matches?.('input[type="search"], input[type="text"], input[type="number"], select, textarea');
    if (e.key === '/' && !typing) {
      e.preventDefault();
      searchEl?.focus();
      searchEl?.select();
    } else if (e.key === 'Escape') {
      if (openKey) {
        openKey = detail = null;
        list?.focus();
      } else if (t === searchEl && view.query) view.query = '';
    }
  }

  const sortHead = (key: string, label: string) => label + (view.sort === key ? (view.desc ? ' ▼' : ' ▲') : '');
  function sortBy(key: string) {
    if (view.sort === key) view.desc = !view.desc;
    else {
      view.sort = key;
      view.desc = key === 'size';
    }
  }
  let allBox: HTMLInputElement | undefined = $state();
  $effect(() => {
    if (allBox) allBox.indeterminate = includedN > 0 && includedN < total;
  });
  const jobBusy = $derived(app.job !== '');
</script>

<svelte:window onkeydown={onWindowKey} />

<section class="games" aria-labelledby="games-h">
  <div class="ghead">
    <div>
      <h2 id="games-h">Games</h2>
      <span class="sub">{plural(counts?.Systems?.reduce((n, s) => n + s.Total, 0) ?? 0, 'game')} in {plural(counts?.Systems?.length ?? 0, 'system')}</span>
    </div>
    {#if outputs.length}
      <div class="for">
        {#if outputs.length > 1}
          <label for="out-pick">Choosing for</label>
          <select id="out-pick" value={outId} onchange={(e) => chooseOutput(Number(e.currentTarget.value))}>
            {#each outputs as o (o.ID)}<option value={o.ID}>{o.Name}</option>{/each}
          </select>
        {:else}
          <span>Choosing for <b>{output?.Name}</b></span>
        {/if}
        <span class="where">{output?.OutputRoot}</span>
      </div>
    {:else}
      <div class="for"><span>Add an output on the Outputs tab to choose which games go where.</span></div>
    {/if}
  </div>

  {#if !app.st?.Sources?.length}
    <p class="empty-note">Add a ROM folder and sync first (Library tab).</p>
  {:else}
    <div class="gbody" style="--rail-w: {railW}px">
      <SystemRail
        {counts}
        {info}
        selected={sysFilter}
        outputName={output?.Name ?? ''}
        hasOutput={!!outId}
        ontick={tickSystem}
        onfilter={filterSystem} />

      <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
      <div
        class="railsep"
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize the systems panel"
        aria-valuenow={Math.round(railW)}
        aria-valuemin={RAIL_MIN}
        aria-valuemax={RAIL_MAX}
        tabindex="0"
        onpointerdown={startRailResize}
        ondblclick={() => ((railW = 216), saveRailW())}
        onkeydown={railResizeKey}></div>

      <div class="listarea" class:one-sys={sysFilter.length === 1}>
        <div class="tools">
          <div class="search">
            <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><circle cx="7" cy="7" r="4.8" /><path d="M10.6 10.6 14 14" stroke-linecap="round" /></svg>
            <input id="q" type="search" placeholder="Search games" aria-label="Search games" autocomplete="off" spellcheck="false" bind:value={view.query} bind:this={searchEl} />
            <kbd aria-hidden="true">/</kbd>
          </div>
          <div class="chips">
            <div class="seg" role="group" aria-label="Show">
              {#each [['all', 'All'], ['included', 'Included'], ['excluded', 'Not included']] as [id, label]}
                <button type="button" aria-pressed={view.show === id} disabled={!outId && id !== 'all'} onclick={() => (view.show = id)}>{label}</button>
              {/each}
            </div>
            <button type="button" class="tog" aria-pressed={view.several} onclick={() => (view.several = !view.several)}>Several versions</button>
            <button type="button" class="tog" aria-pressed={view.choices} onclick={() => (view.choices = !view.choices)}>Your choices</button>
            <button type="button" class="tog" aria-pressed={view.unverified} onclick={() => (view.unverified = !view.unverified)}>Not verified</button>
            {#if anyFilter}<button type="button" class="link" onclick={clearFilters}>Clear filters</button>{/if}
          </div>
        </div>

        <div class="listwrap">
          <div class="lhead">
            <span class="c-box"><input type="checkbox" bind:this={allBox} checked={total > 0 && includedN === total} disabled={!outId || total === 0}
              aria-label={includedN === total ? `Leave out all ${num(total)} shown games` : `Include all ${num(total)} shown games`}
              onclick={(e) => { e.preventDefault(); tickShown(); }} /></span>
            <span class="lh-name">
              <button type="button" class="sortb" aria-pressed={view.sort === 'name'} onclick={() => sortBy('name')}>{sortHead('name', 'Game')}</button>
              <span class="shown">
                {num(total)} shown{outId ? ` · ${num(includedN)} included` : ''}
                {#if other}· <button type="button" class="link" onclick={() => (sysFilter = [])}>{plural(other, 'more match', 'more matches')} in other systems</button>{/if}
              </span>
            </span>
            <button type="button" class="sortb c-sys" aria-pressed={view.sort === 'system'} onclick={() => sortBy('system')}>{sortHead('system', 'System')}</button>
            <span class="c-ver">Version</span>
            <span class="c-id" title="How the game was identified">ID</span>
            <button type="button" class="sortb c-size" aria-pressed={view.sort === 'size'} onclick={() => sortBy('size')}>{sortHead('size', 'Size')}</button>
          </div>

          {#if loaded && total === 0}
            <div class="empty">
              <p>{applied ? `No games match “${applied}”${sysFilter.length ? ' in ' + sysFilter.map(short).join(', ') : ''}.` : 'No games fit these filters.'}</p>
              <div class="acts">
                {#if sysFilter.length && other}<button type="button" onclick={() => (sysFilter = [])}>Search all systems</button>{/if}
                {#if anyFilter || sysFilter.length}<button type="button" onclick={() => { clearFilters(); sysFilter = []; }}>Clear filters</button>{/if}
              </div>
            </div>
          {:else}
            <VirtualList bind:this={list} {total} {onrange} {onkey} activeIndex={active} label="Games" row={rowView} />
          {/if}

          {#if detail && openKey}
            <GameDrawer
              {detail}
              outputId={outId}
              {info}
              {counts}
              busy={jobBusy}
              onclose={() => { openKey = detail = null; list?.focus(); }}
              ontick={drawerTick}
              onfollow={follow}
              onversion={version}
              onauto={() => version(null)} />
          {/if}

          {#if toast}
            <div class="toast" role="status">
              <span>{toast.msg}</span>
              <span class="acts">
                {#if toast.action}<button type="button" onclick={() => { const a = toast!.action!; toast = null; a.run(); }}>{toast.action.label}</button>{/if}
                {#if toast.token}<button type="button" onclick={() => { toast = null; undo(); }}>Undo</button>{/if}
              </span>
            </div>
          {/if}
        </div>
      </div>
    </div>

    <ChangesBar {pending} outputName={output?.Name ?? ''} {others} job={app.job} progress={app.progress} onundo={undo} onapply={apply} onapplyall={applyAll} />
  {/if}
</section>

{#snippet rowView(i: number)}
  {@const r = rowAt(i)}
  {#if r}
    {@const c = confidence[r.Confidence] ?? confidence.unidentified}
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_interactive_supports_focus -->
    <div class="row" class:active={i === active} class:open={r.Key === openKey} role="row" id="row-{i}" aria-selected={r.Key === openKey}
      onclick={(e) => { active = i; if (!(e.target as HTMLElement).closest('input')) open(r.Key); list?.focus(); }}>
      <span class="c-box" role="gridcell">
        <input type="checkbox" tabindex="-1" checked={r.Included} disabled={!outId} aria-label="Include {r.Name}"
          onclick={(e) => { e.preventDefault(); onRowTick(i, e.shiftKey); }} />
        {#if r.Explicit && outId}<span class="mine" title="{r.Included ? 'Ticked' : 'Unticked'} by you"></span>{/if}
      </span>
      <span class="c-name" role="gridcell">
        <span class="t">{#each highlight(r.Name, applied) as [text, hit]}{#if hit}<mark>{text}</mark>{:else}{text}{/if}{/each}</span>
        {#if r.Discs > 1}<span class="badge">{r.Discs} discs</span>{/if}
        {#if r.Versions > 1}<span class="badge">{r.Versions} versions</span>{/if}
        {#if r.VersionPicked}<span class="badge pin">your version</span>{/if}
      </span>
      <span class="c-sys" role="gridcell">{short(r.SystemID)}</span>
      <span class="c-ver" role="gridcell">{r.Version}</span>
      <span class="c-id {c.cls}" role="gridcell" title={c.label}>{c.icon}</span>
      <span class="c-size" role="gridcell" title={r.SizeEstimated ? 'Estimate: converted when applied' : ''}>{size(r.Size, r.SizeEstimated)}</span>
    </div>
  {:else}
    <div class="row skeleton" role="row" id="row-{i}"><span class="c-box"></span><span class="c-name"><span class="t muted">…</span></span></div>
  {/if}
{/snippet}
