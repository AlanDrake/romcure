<script lang="ts">
  import type {SystemCounts} from '../../../bindings/romcure/internal/app/models';
  import {num, words} from '../../lib/games';

  let {
    counts,
    info,
    selected,
    outputName,
    hasOutput,
    ontick,
    onfilter,
  }: {
    counts: SystemCounts | null;
    info: Record<string, {short: string; name: string}>;
    selected: string[];
    outputName: string;
    hasOutput: boolean;
    ontick: (systemId: string | null, included: boolean) => void;
    onfilter: (systemId: string | null, multi: boolean) => void;
  } = $props();

  const systems = $derived(counts?.Systems ?? []);
  const total = $derived(systems.reduce((n, s) => n + s.Total, 0));
  const included = $derived(systems.reduce((n, s) => n + s.Included, 0));

  // What a new game does here, in words.
  const rule = $derived.by(() => {
    if (!counts || !hasOutput) return '';
    const on: string[] = [];
    const off: string[] = [];
    for (const s of systems) {
      const v = counts.Ticked?.[s.SystemID] ?? counts.Default;
      (v ? on : off).push(info[s.SystemID]?.short ?? s.SystemID);
    }
    if (counts.Default) return off.length ? `New games are added automatically, except to ${words(off)}.` : 'New games are added automatically.';
    return on.length
      ? `New games are added to ${words(on)}. Elsewhere they are left out until you tick them.`
      : 'New games are left out until you tick them.';
  });

  // A checkbox that can show the "some" state.
  function tri(node: HTMLInputElement, partial: boolean) {
    node.indeterminate = partial;
    return {
      update(p: boolean) {
        node.indeterminate = p;
      },
    };
  }
</script>

<aside class="rail" aria-label="Systems">
  <div class="list">
    <div class="sys root" class:on={selected.length === 0}>
      <input
        type="checkbox"
        aria-label="Include every game in {outputName}"
        disabled={!hasOutput}
        checked={total > 0 && included === total}
        use:tri={included > 0 && included < total}
        onclick={(e) => { e.preventDefault(); ontick(null, included !== total); }} />
      <button type="button" class="nm" onclick={() => onfilter(null, false)}><b>All systems</b></button>
      <span class="cnt">{num(included)}/{num(total)}</span>
    </div>
    {#each systems as s (s.SystemID)}
      {@const i = info[s.SystemID]}
      <div class="sys" class:on={selected.includes(s.SystemID)}>
        <input
          type="checkbox"
          aria-label="Include every {i?.short ?? s.SystemID} game in {outputName}"
          disabled={!hasOutput}
          checked={s.Total > 0 && s.Included === s.Total}
          use:tri={s.Included > 0 && s.Included < s.Total}
          onclick={(e) => { e.preventDefault(); ontick(s.SystemID, s.Included !== s.Total); }} />
        <button type="button" class="nm" onclick={(e) => onfilter(s.SystemID, e.ctrlKey || e.metaKey)}>
          <b>{i?.short ?? s.SystemID}</b><small>{i?.name ?? ''}</small>
        </button>
        <span class="cnt">{num(s.Included)}/{num(s.Total)}</span>
      </div>
    {/each}
  </div>
  {#if rule}<p class="rule">{rule}</p>{/if}
</aside>
