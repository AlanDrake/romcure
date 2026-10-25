<script lang="ts">
  import type {Snippet} from 'svelte';

  // A windowed list of fixed-height rows: only the rows in view (plus a few
  // around them) exist in the page, so 100k games scroll smoothly.
  let {
    total,
    rowHeight = 34,
    row,
    onrange,
    onkey,
    label,
    activeIndex = -1,
  }: {
    total: number;
    rowHeight?: number;
    row: Snippet<[number]>;
    onrange: (start: number, end: number) => void;
    onkey?: (e: KeyboardEvent) => void;
    label: string;
    activeIndex?: number;
  } = $props();

  const OVERSCAN = 10;
  let el: HTMLDivElement;
  let top = $state(0);
  let height = $state(400);

  const start = $derived(Math.max(0, Math.floor(top / rowHeight) - OVERSCAN));
  const end = $derived(Math.min(total, Math.ceil((top + height) / rowHeight) + OVERSCAN));
  const indexes = $derived(Array.from({length: Math.max(0, end - start)}, (_, i) => start + i));

  $effect(() => {
    onrange(start, end);
  });

  export function scrollToTop() {
    if (el) el.scrollTop = 0;
  }
  export function scrollToIndex(i: number) {
    if (!el) return;
    const t = i * rowHeight;
    if (t < el.scrollTop) el.scrollTop = t;
    else if (t + rowHeight > el.scrollTop + el.clientHeight) el.scrollTop = t + rowHeight - el.clientHeight;
  }
  export function focus() {
    el?.focus({preventScroll: true});
  }
</script>

<div
  class="vlist"
  bind:this={el}
  bind:clientHeight={height}
  onscroll={() => (top = el.scrollTop)}
  onkeydown={onkey}
  tabindex="0"
  role="grid"
  aria-label={label}
  aria-rowcount={total}
  aria-activedescendant={activeIndex >= start && activeIndex < end ? 'row-' + activeIndex : undefined}>
  <div class="vspacer" style="height:{total * rowHeight}px">
    <div class="vrows" style="transform:translateY({start * rowHeight}px)" role="rowgroup">
      {#each indexes as i (i)}{@render row(i)}{/each}
    </div>
  </div>
</div>
