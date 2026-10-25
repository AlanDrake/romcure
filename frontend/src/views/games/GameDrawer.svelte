<script lang="ts">
  import type {GameDetail, SystemCounts} from '../../../bindings/romcure/internal/app/models';
  import {confidence} from '../../lib/games';

  let {
    detail,
    outputId,
    info,
    counts,
    busy,
    onclose,
    ontick,
    onfollow,
    onversion,
    onauto,
  }: {
    detail: GameDetail;
    outputId: number;
    info: Record<string, {short: string; name: string}>;
    counts: SystemCounts | null;
    busy: boolean;
    onclose: () => void;
    ontick: (outputId: number, included: boolean) => void;
    onfollow: () => void;
    onversion: (gameKey: string) => void;
    onauto: () => void;
  } = $props();

  const sys = $derived(info[detail.SystemID]?.short ?? detail.SystemID);
  const group = $derived(detail.Groups?.[0]);
  const picked = $derived(group?.Variants?.find((v) => v.Selected));
  const here = $derived(detail.Outputs?.find((o) => o.ID === outputId));
  const others = $derived((detail.Outputs ?? []).filter((o) => o.ID !== outputId));
  const overridden = $derived(!!detail.Groups?.some((g) => g.Overridden));
  const grouping: Record<string, string> = {
    dat_pc: "By the DAT's parent/clone list",
    clone_list: 'By a clone list',
    title_normalization: 'By title (guessed)',
    none: 'Not grouped',
  };

  function why(o: {Name: string; Included: boolean; Source: string}): string {
    if (o.Source === 'game') return o.Included ? 'Ticked by you.' : 'Unticked by you.';
    if (o.Source === 'system') {
      return o.Included ? `Included because ${sys} is ticked. New ${sys} games are added too.` : `Left out because ${sys} is unticked.`;
    }
    return o.Included ? `Included: ${o.Name} takes every game unless you untick it.` : `Left out: ${o.Name} only takes the games and systems you tick.`;
  }
  const whyShort = (o: {Included: boolean; Source: string}) =>
    o.Source === 'game' ? (o.Included ? 'ticked by you' : 'unticked by you') : o.Source === 'system' ? `${sys} is ${o.Included ? 'ticked' : 'unticked'}` : o.Included ? 'takes every game' : 'only what you tick';
  const tags = (v: any) => [v.Region, v.Language, v.Revision].filter(Boolean).join(' · ');
</script>

<aside class="drawer" aria-label="Game details">
  <div class="dh">
    <div>
      <h3>{detail.Name}</h3>
      <p>{info[detail.SystemID]?.name ?? detail.SystemID}{detail.Discs > 1 ? ` · ${detail.Discs} discs` : ''}</p>
    </div>
    <button type="button" class="x" onclick={onclose} aria-label="Close details">×</button>
  </div>

  {#if here}
    <section>
      <h4>On {here.Name}</h4>
      <label class="tick"><input type="checkbox" checked={here.Included} onchange={(e) => ontick(here.ID, e.currentTarget.checked)} /> Include this game</label>
      <p class="why">{why(here)}</p>
      {#if here.Source === 'game'}<button type="button" class="link" onclick={onfollow}>Follow the {sys} setting instead</button>{/if}
    </section>
  {/if}

  {#if others.length}
    <section>
      <h4>Other outputs</h4>
      {#each others as o (o.ID)}
        <label class="tick">
          <input type="checkbox" checked={o.Included} onchange={(e) => ontick(o.ID, e.currentTarget.checked)} />
          {o.Name} <small>{whyShort(o)}</small>
        </label>
      {/each}
    </section>
  {/if}

  {#if group}
    <section>
      <h4>Version <span>· used on every output</span></h4>
      <div class="vers" role="radiogroup" aria-label="Version">
        {#each group.Variants ?? [] as v (v.GameKey)}
          <label class="ver" class:on={v.Selected}>
            <input type="radio" name="ver" checked={v.Selected} disabled={busy} onchange={() => onversion(v.GameKey)} />
            <span class="vt">{v.Title}</span>
            <span class="vm">{tags(v)}{tags(v) ? ' · ' : ''}{confidence[v.Confidence]?.label.toLowerCase() ?? v.Confidence}{v.BetaProtoDemo ? ' · beta/proto/demo' : ''}{v.Selected && !overridden ? ' · ' : ''}{#if v.Selected && !overridden}<b>automatic pick</b>{/if}</span>
          </label>
        {/each}
      </div>
      <p class="why">{group.SelectionReason}{detail.Discs > 1 ? ' Every disc uses the matching version.' : ''}</p>
      {#if overridden}<button type="button" class="link" onclick={onauto} disabled={busy}>Back to automatic</button>{/if}
    </section>

    <section>
      <h4>Details</h4>
      <dl class="facts">
        <dt>Identified</dt><dd>{picked ? (confidence[picked.Confidence]?.label ?? picked.Confidence) : ''}</dd>
        <dt>Grouped</dt><dd>{grouping[group.GroupingSource] ?? group.GroupingSource}</dd>
        {#if picked}<dt>Source</dt><dd class="path">{picked.SourcePath}{picked.MemberName ? ' › ' + picked.MemberName : ''}</dd>{/if}
      </dl>
    </section>
  {/if}
</aside>
