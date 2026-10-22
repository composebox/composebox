<script>
  import { tick } from 'svelte';

  let {
    open = false,
    action = '',
    logs = '',
    status = 'running',
    onClose = () => {},
    onClear = () => {},
  } = $props();

  let minimized = $state(false);
  let termEl = $state(null);

  $effect(() => {
    if (logs && termEl && !minimized) {
      tick().then(() => {
        if (termEl) termEl.scrollTop = termEl.scrollHeight;
      });
    }
  });
</script>

{#if open}
  <div
    class="border-t border-zinc-800 bg-zinc-950 flex flex-col shrink-0 {minimized
      ? 'h-9'
      : 'h-64 sm:h-72'} transition-all duration-150 shadow-2xl"
  >
    <!-- Header bar -->
    <div
      class="h-9 px-3.5 border-b border-zinc-800/80 bg-zinc-900/70 flex items-center justify-between shrink-0 select-none"
    >
      <div class="flex items-center gap-2">
        <svg
          class="h-3.5 w-3.5 text-zinc-400"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <polyline points="4 17 10 11 4 5" /><line x1="12" y1="19" x2="20" y2="19" />
        </svg>
        <span class="text-xs font-medium text-zinc-200">
          Action Console: <span class="font-mono text-zinc-400">{action || 'idle'}</span>
        </span>

        {#if status === 'running'}
          <span
            class="inline-flex items-center gap-1 text-[10px] text-amber-400 bg-amber-500/10 border border-amber-500/20 px-1.5 py-0.2 rounded-full font-mono"
          >
            <span class="h-1.5 w-1.5 rounded-full bg-amber-400 animate-pulse"></span>
            executing...
          </span>
        {:else if status === 'success'}
          <span
            class="inline-flex items-center gap-1 text-[10px] text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 px-1.5 py-0.2 rounded-full font-mono"
          >
            <span class="h-1.5 w-1.5 rounded-full bg-emerald-400"></span>
            completed
          </span>
        {:else if status === 'error'}
          <span
            class="inline-flex items-center gap-1 text-[10px] text-red-400 bg-red-500/10 border border-red-500/20 px-1.5 py-0.2 rounded-full font-mono"
          >
            <span class="h-1.5 w-1.5 rounded-full bg-red-400"></span>
            failed
          </span>
        {:else if status === 'idle'}
          <span
            class="inline-flex items-center gap-1 text-[10px] text-zinc-400 bg-zinc-800/60 border border-zinc-700/60 px-1.5 py-0.2 rounded-full font-mono"
          >
            <span class="h-1.5 w-1.5 rounded-full bg-zinc-500"></span>
            ready
          </span>
        {/if}
      </div>

      <div class="flex items-center gap-1">
        {#if !minimized && (logs || status !== 'idle')}
          <button
            onclick={onClear}
            class="px-2 py-0.5 text-[11px] text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800 rounded transition-colors"
            title="Clear Output"
          >
            Clear
          </button>
        {/if}
        <button
          onclick={() => (minimized = !minimized)}
          class="p-1 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800 rounded transition-colors"
          title={minimized ? 'Expand Console' : 'Minimize Console'}
        >
          {#if minimized}
            <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="18 15 12 9 6 15" />
            </svg>
          {:else}
            <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="6 9 12 15 18 9" />
            </svg>
          {/if}
        </button>
        <button
          onclick={onClose}
          class="p-1 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800 rounded transition-colors"
          title="Close Console"
        >
          <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M18 6 6 18"/><path d="m6 6 12 12"/>
          </svg>
        </button>
      </div>
    </div>

    <!-- Output terminal -->
    {#if !minimized}
      <div
        bind:this={termEl}
        class="flex-1 p-3.5 font-mono text-xs leading-relaxed text-zinc-300 overflow-y-auto whitespace-pre-wrap select-text bg-zinc-950/95"
      >
        {logs || 'No action executed yet. Output from Update, Start, Restart, and Stop actions will stream here.'}
      </div>
    {/if}
  </div>
{/if}
