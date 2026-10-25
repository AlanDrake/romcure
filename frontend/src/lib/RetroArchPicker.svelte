<script lang="ts">
  import {onMount} from 'svelte';
  import * as Service from '../../bindings/romcure/cmd/romcure/service';
  import type {RetroArchInstall} from '../../bindings/romcure/internal/discover/models';

  // The retroarch.cfg of the RetroArch to use: found on this computer or picked by hand.
  // "" means none chosen. onpick runs when the user changes it.
  let {value = $bindable(''), onpick}: {value?: string; onpick?: (config: string) => void} = $props();

  let installs: RetroArchInstall[] = $state([]);
  let loaded = $state(false);
  let error = $state('');

  const message = (e: unknown) => (e instanceof Error ? e.message : String(e)).replace(/^.*?Error:\s*/, '');
  const known = $derived(installs.some((i) => i.Config === value));

  async function find() {
    try {
      installs = (await Service.RetroArchInstalls()) ?? [];
      // A first look chooses the running or last-used one, which the user can change.
      if (!value && installs.length) choose(installs[0].Config);
    } catch (e) {
      error = message(e);
    }
    loaded = true;
  }
  function choose(config: string) {
    value = config;
    onpick?.(config);
  }
  async function browse() {
    error = '';
    try {
      const found = await Service.PickRetroArch();
      if (!found) return;
      if (!installs.some((i) => i.Config === found.Config)) installs = [...installs, found];
      choose(found.Config);
    } catch (e) {
      error = message(e);
    }
  }
  const when = (t: string) => (t && !t.startsWith('0001') ? new Date(t).toLocaleDateString() : '');

  onMount(find);
</script>

<div class="ra-picker">
  <p class="muted">Which RetroArch is this output for? Its playlists will be added to the ones RetroArch already has.</p>
  <div class="list" role="radiogroup" aria-label="RetroArch">
    {#each installs as i}
      <label class="pick">
        <input type="radio" name="retroarch" checked={value === i.Config} onchange={() => choose(i.Config)} />
        <span>
          <b>{i.Dir}</b>
          <span class="muted">
            {#if i.Running}Open now. Close it before pressing Apply.{:else if when(i.Used)}Last used {when(i.Used)}.{/if}
            Playlists go to {i.Playlists}
          </span>
        </span>
      </label>
    {/each}
    {#if value && !known}
      <label class="pick">
        <input type="radio" name="retroarch" checked onchange={() => {}} />
        <span><b>{value}</b><span class="muted">Chosen earlier; not found now.</span></span>
      </label>
    {/if}
    <label class="pick">
      <input type="radio" name="retroarch" checked={!value} onchange={() => choose('')} />
      <span><b>Not now</b><span class="muted">No playlists are made until you choose one.</span></span>
    </label>
  </div>
  {#if loaded && !installs.length}
    <p class="muted">No RetroArch was found. If it is installed, choose its <code>retroarch.cfg</code> or <code>retroarch.exe</code>.</p>
  {/if}
  <div class="form">
    <button type="button" onclick={browse}>Choose RetroArch…</button>
    <button type="button" class="quiet" onclick={find}>Look again</button>
  </div>
  {#if error}<p class="bad" role="alert">{error}</p>{/if}
</div>

<style>
  .list { display: flex; flex-direction: column; gap: 4px; margin: 8px 0; }
  .pick { display: flex; gap: 10px; align-items: flex-start; padding: 8px 10px; margin: 0; border: 1px solid var(--line); border-radius: 8px; cursor: pointer; }
  .pick:hover { background: var(--hover); }
  .pick span { display: flex; flex-direction: column; min-width: 0; overflow-wrap: anywhere; }
  .bad { color: var(--bad); }
</style>
