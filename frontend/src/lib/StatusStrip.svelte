<script lang="ts">
  import * as Service from '../../bindings/romcure/cmd/romcure/service';
  import {app, jobLabel} from './state.svelte';

  let now = $state(Date.now());
  let cancelling = $state(false);

  // Tick once a second while a job runs, so the elapsed time moves.
  $effect(() => {
    if (!app.job) {
      cancelling = false;
      return;
    }
    if (!app.jobStarted) app.jobStarted = Date.now();
    const t = setInterval(() => (now = Date.now()), 500);
    return () => clearInterval(t);
  });

  const short = (id: string) => {
    for (const s of app.st?.Systems ?? []) if (s.ID === id) return s.Short;
    for (const s of app.st?.Sources ?? []) if (s.SystemID === id) return s.Short;
    return id;
  };

  const what = $derived.by(() => {
    const s = app.step;
    const sys = s.SystemID ? short(s.SystemID) : '';
    switch (s.Stage) {
      case 'scan': return `Reading your ${sys} folder`;
      case 'identify': return `Identifying ${sys} games`;
      case 'generate': return `Updating ${s.Profile}${sys ? ' · ' + sys : ''}`;
      case 'convert': return `Converting ${s.Item}${s.Percent ? ' · ' + Math.round(s.Percent) + '%' : ''}`;
      case 'verify': return `Checking ${s.Profile}`;
      case 'dat': return `Updating the ${s.Detail} database for ${sys}`;
      case 'chd': return `Checking ${s.Item}${s.Total ? ` (${s.Done + 1} of ${s.Total})` : ''}`;
      default: return app.progress;
    }
  });

  // The file or game being handled right now (conversions and CHD checks already name theirs above).
  const item = $derived(app.step.Stage === 'scan' || app.step.Stage === 'generate' ? app.step.Item : '');

  // How far along the whole job is; a running conversion counts partially.
  const fraction = $derived.by(() => {
    const s = app.step;
    if (!s.Total) return null;
    const part = s.Stage === 'convert' ? s.Percent / 100 : 0;
    return Math.min(1, (s.Done + part) / s.Total);
  });

  // "3/20": the step being worked on out of all steps (a finished job is gone).
  const count = $derived(app.step.Total ? `${Math.min(app.step.Done + 1, app.step.Total)}/${app.step.Total}` : '');

  const elapsed = $derived.by(() => {
    const secs = Math.max(0, Math.floor((now - app.jobStarted) / 1000));
    const m = Math.floor(secs / 60);
    return `${m}:${String(secs % 60).padStart(2, '0')}`;
  });

  function cancel() {
    cancelling = true;
    Service.CancelJob();
  }
</script>

{#if app.job}
  <div class="strip" role="status" aria-live="polite">
    <div class="line">
      <b>{jobLabel[app.job] ?? 'Working'}</b>
      <span class="what">{what}</span>
      {#if count}<span class="count" title="Step {count}">{count}</span>{/if}
      <span class="time">{elapsed}</span>
      <button type="button" class="quiet" onclick={cancel} disabled={cancelling}>{cancelling ? 'Cancelling…' : 'Cancel'}</button>
    </div>
    <div class="itemline" title={item}>{item || ' '}</div>
    <div class="meter" class:indeterminate={fraction === null}>
      <i style={fraction === null ? '' : `width:${(fraction * 100).toFixed(1)}%`}></i>
    </div>
  </div>
{/if}
