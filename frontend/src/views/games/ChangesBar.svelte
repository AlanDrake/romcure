<script lang="ts">
  import type {Pending} from '../../../bindings/romcure/internal/app/models';
  import {num, plural, size, words} from '../../lib/games';

  let {
    pending,
    outputName,
    others,
    job,
    progress,
    onundo,
    onapply,
    onapplyall,
  }: {
    pending: Pending | null;
    outputName: string;
    others: string[]; // other outputs that also have changes waiting
    job: string;
    progress: string;
    onundo: () => void;
    onapply: () => void;
    onapplyall: () => void;
  } = $props();

  const dev = $derived(pending?.Device ?? null);
  const blocked = $derived(!!dev?.Blockers?.length);
  const fill = $derived(dev && dev.FreeBytes > 0 ? Math.min(1, (pending?.Bytes ?? 0) / dev.FreeBytes) : 0);
  const busy = $derived(job === 'apply' || job === 'sync');
  const parts = $derived.by(() => {
    if (!pending) return [];
    const p: string[] = [];
    if (pending.Add) p.push(`+${num(pending.Add)} to add`);
    if (pending.Remove) p.push(`−${num(pending.Remove)} to remove`);
    if (pending.Replace) p.push(plural(pending.Replace, 'version change'));
    return p;
  });
</script>

<footer class="bar">
  <div class="state">
    <div class="l1">
      {#if busy}
        <b>Updating {outputName}…</b> <span class="muted">{progress}</span>
      {:else if !pending}
        <span class="muted">Choose an output to start.</span>
      {:else if pending.Any}
        <b>{parts.join(' · ')}</b> <span class="muted">not applied to {outputName} yet</span>
      {:else}
        <span class="good">✓</span> {outputName} is up to date <span class="muted">· {plural(pending.Count, 'game')}</span>
      {/if}
    </div>
    {#if dev && !busy}
      <div class="meter" class:over={blocked} class:warn={!blocked && fill > 0.9} role="img" aria-label="{Math.round(fill * 100)}% of the free space">
        <i style="width:{(fill * 100).toFixed(1)}%"></i>
      </div>
    {/if}
    <div class="l2">
      {#if dev && !busy}
        {#if blocked}
          {#each dev.Blockers ?? [] as b}<span class="bad">{b}</span>{/each}
        {:else}
          <span>Needs {size(pending?.Bytes ?? 0, pending?.Estimated)}</span>
          <span>{size(dev.FreeBytes)} free · {dev.FSType}</span>
        {/if}
        {#if dev.Oversize?.length}<span class="bad">{plural(dev.Oversize.length, 'game')} over the {size(dev.MaxFileBytes)} file limit will be skipped</span>{/if}
      {/if}
      {#if others.length && !busy}
        <span>Also changed: {words(others)} · <button type="button" class="link" onclick={onapplyall}>Apply all</button></span>
      {/if}
    </div>
  </div>
  <div class="acts">
    <button type="button" onclick={onundo} disabled={!pending?.UndoLabel || busy} title={pending?.UndoLabel ? 'Undo: ' + pending.UndoLabel : ''}>Undo</button>
    <button type="button" class="primary" onclick={onapply} disabled={!pending?.Any || blocked || busy}>Apply to {outputName}</button>
  </div>
</footer>
