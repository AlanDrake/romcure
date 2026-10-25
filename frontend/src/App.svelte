<script lang="ts">
  import {onMount} from 'svelte';
  import {Events} from '@wailsio/runtime';
  import {app, refresh, fail, ok, resetStep, relaunchElevated, keepCopies, followLink, type Tab} from './lib/state.svelte';
  import Setup from './views/Setup.svelte';
  import StatusStrip from './lib/StatusStrip.svelte';
  import Library from './views/Library.svelte';
  import Games from './views/Games.svelte';
  import Outputs from './views/Outputs.svelte';
  import Settings from './views/Settings.svelte';
  import Help from './views/Help.svelte';

  const tabs: [Tab, string][] = [
    ['library', 'Library'],
    ['games', 'Games'],
    ['outputs', 'Outputs'],
    ['settings', 'Settings'],
    ['help', 'Help'],
  ];

  // A fresh install (nothing set up, setup never finished or skipped) starts with the
  // setup, which then stays open until it is finished or skipped, even though the
  // first folder it adds makes the install look "not fresh".
  $effect(() => {
    if (app.st && !app.st.Settings.onboarded && !app.st.Sources?.length && !app.st.Profiles?.length) app.setupOpen = true;
  });
  const showSetup = $derived(app.setupOpen);

  onMount(() => {
    refresh();
    const offP = Events.On('job:progress', (ev: any) => {
      const p = ev.data;
      app.progress = [p.Stage, p.SystemID, p.Profile, p.Detail].filter(Boolean).join(' · ');
      app.step = p;
    });
    const offD = Events.On('job:done', (ev: any) => {
      const d = ev.data;
      app.job = '';
      app.progress = '';
      app.jobStarted = 0;
      resetStep();
      if (d.Kind === 'dats') app.datResults = d.DATs ?? [];
      if (d.Kind === 'chd') app.chdResults = d.CHDs ?? [];
      if (d.Kind === 'sync' || d.Kind === 'apply') app.syncDone++;
      const what = ({sync: 'Sync', apply: 'Update', dats: 'Database update', chd: 'CHD check'} as Record<string, string>)[d.Kind] ?? 'Job';
      const problems = ((d.Report?.Profiles ?? []) as any[]).reduce((n, r) => n + (r.Errors?.length ?? 0) + (r.Blockers?.length ?? 0), 0);
      const seeOutputs = {label: 'See the outputs', tab: 'outputs' as Tab};
      if (d.Cancelled) ok(what + ' cancelled.');
      else if (d.Error) fail(d.Error);
      else if (d.Kind === 'sync' && problems) ok(`Sync finished with ${problems} ${problems === 1 ? 'problem' : 'problems'}.`, seeOutputs, true);
      else if (d.Kind === 'sync' && d.Report?.Unreadable?.length) {
        const n = d.Report.Unreadable.length;
        ok(`Sync finished, but ${n} ${n === 1 ? 'file' : 'files'} could not be read and ${n === 1 ? 'was' : 'were'} skipped.`, {label: 'See which', tab: 'library', anchor: 'unreadable'}, true);
      } else if (d.Kind === 'sync') ok('Sync finished.', {label: 'See the report', tab: 'library'});
      else if (d.Kind === 'apply' && problems) ok(`Outputs updated with ${problems} ${problems === 1 ? 'problem' : 'problems'}.`, seeOutputs, true);
      else if (d.Kind === 'apply') ok('Outputs are up to date.');
      else if (d.Kind === 'dats' || d.Kind === 'chd') {
        // The link scrolls to the list under the button that started the job (none if it came back empty).
        const [list, anchor] = d.Kind === 'dats' ? [app.datResults, 'dat-results'] : [app.chdResults, 'chd-results'];
        ok(what + ' finished. Run Sync to apply the results to your library.', list.length ? {label: 'See the results', tab: 'library', anchor} : null);
      } else ok(what + ' finished.');
      refresh();
    });
    return () => {
      offP?.();
      offD?.();
    };
  });
</script>

{#if showSetup}
  <StatusStrip />
  <Setup />
{:else}
<div class="shell">
  <nav>
    <h1>RomCure</h1>
    {#each tabs as [id, label]}
      <button class:active={app.tab === id} onclick={() => (app.tab = id)}>{label}</button>
    {/each}
  </nav>

  <main class:wide={app.tab === 'games'}>
    <StatusStrip />
    {#if app.error}<div class="msg err" role="alert">{app.error}</div>{/if}
    {#if app.notice}
      <div class="msg" class:ok={!app.noticeWarn} class:warn={app.noticeWarn}>
        {app.notice}
        {#if app.noticeLink}<button class="link" onclick={() => followLink(app.noticeLink!)}>{app.noticeLink.label}</button>{/if}
      </div>
    {/if}
    {#if app.st?.Warning}<div class="msg warn">{app.st.Warning}</div>{/if}
    {#if app.st?.Links?.Offer}
      <div class="msg warn" id="link-offer">
        <b>Links need administrator rights on this PC.</b> Without them, outputs are copied instead, which uses more disk
        space.
        <div class="form">
          <button onclick={relaunchElevated}>Restart as administrator</button>
          <button class="quiet" onclick={keepCopies}>Keep using copies</button>
        </div>
      </div>
    {/if}

    {#if !app.st}
      <p class="muted">Loading…</p>
    {:else if app.tab === 'library'}
      <Library />
    {:else if app.tab === 'games'}
      <Games />
    {:else if app.tab === 'outputs'}
      <Outputs />
    {:else if app.tab === 'settings'}
      <Settings />
    {:else}
      <Help />
    {/if}
  </main>
</div>
{/if}
