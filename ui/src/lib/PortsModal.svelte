<script>
  import { onMount } from 'svelte';
  import { fade, scale } from 'svelte/transition';
  import { Network, Search, RefreshCw, X, AlertCircle, ArrowUpRight } from '@lucide/svelte';
  import { getPorts } from './api';

  let { isOpen = false, onClose, onSelectStack } = $props();

  let loading = $state(true);
  let error = $state(null);
  let allocations = $state([]);
  let search = $state('');

  async function loadData() {
    loading = true;
    error = null;
    try {
      const res = await getPorts();
      allocations = res.allocations || [];
    } catch (err) {
      error = err.message || 'Failed to load port allocations';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (isOpen) {
      loadData();
      search = '';
    }
  });

  let filteredAllocs = $derived(
    search.trim() === ''
      ? allocations
      : allocations.filter((a) => {
          const q = search.toLowerCase();
          return (
            String(a.port).includes(q) ||
            String(a.container_port).includes(q) ||
            a.stack.toLowerCase().includes(q) ||
            a.service.toLowerCase().includes(q) ||
            a.host_ip.toLowerCase().includes(q)
          );
        })
  );

  function handleKeydown(e) {
    if (e.key === 'Escape' && isOpen) {
      onClose();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if isOpen}
  <!-- Backdrop -->
  <div
    role="presentation"
    onclick={onClose}
    transition:fade={{ duration: 150 }}
    class="fixed inset-0 z-50 bg-black/75 backdrop-blur-xs flex items-center justify-center p-4"
  >
    <!-- Modal Card -->
    <div
      role="dialog"
      aria-modal="true"
      tabindex="-1"
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      transition:scale={{ duration: 150, start: 0.98 }}
      class="relative w-full max-w-3xl max-h-[90vh] overflow-hidden rounded-xl border border-zinc-800 bg-zinc-900 shadow-2xl flex flex-col text-zinc-100"
    >
      <!-- Header -->
      <div class="px-5 py-4 border-b border-zinc-800 flex items-center justify-between shrink-0 bg-zinc-900/90">
        <div class="flex items-center gap-2.5">
          <div class="p-2 rounded-lg bg-indigo-500/10 border border-indigo-500/20 text-indigo-400">
            <Network class="h-4 w-4" />
          </div>
          <div>
            <h2 class="text-sm font-semibold text-zinc-100">Host Port Allocation Matrix</h2>
            <p class="text-xs text-zinc-400">All host network port bindings across declared stacks</p>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <button
            onclick={loadData}
            disabled={loading}
            title="Refresh port mappings"
            class="p-1.5 rounded-md text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800 disabled:opacity-40 transition-colors cursor-pointer"
          >
            <RefreshCw class="h-4 w-4 {loading ? 'animate-spin' : ''}" />
          </button>
          <button
            onclick={onClose}
            class="p-1.5 rounded-md text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800 transition-colors cursor-pointer"
          >
            <X class="h-4 w-4" />
          </button>
        </div>
      </div>

      <!-- Search Bar -->
      <div class="p-3.5 border-b border-zinc-800/80 bg-zinc-950/40 flex items-center justify-between gap-3 shrink-0">
        <div class="relative flex-1 max-w-md">
          <Search class="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-zinc-500" />
          <input
            type="text"
            bind:value={search}
            placeholder="Filter by port, stack, or service..."
            class="w-full pl-8 pr-3 py-1.5 text-xs rounded-md bg-zinc-900 border border-zinc-800 text-zinc-200 placeholder-zinc-500 focus:outline-none focus:border-indigo-500/50"
          />
        </div>
        <div class="text-[11px] font-mono text-zinc-500">
          {filteredAllocs.length} stack port{filteredAllocs.length === 1 ? '' : 's'} declared
        </div>
      </div>

      <!-- Table Body -->
      <div class="flex-1 overflow-y-auto">
        {#if error}
          <div class="m-5 p-3 rounded-lg bg-red-500/10 border border-red-500/30 flex items-start gap-2.5 text-xs text-red-400">
            <AlertCircle class="h-4 w-4 shrink-0 mt-0.5" />
            <div class="flex-1">{error}</div>
          </div>
        {/if}

        {#if loading && allocations.length === 0}
          <div class="py-12 flex flex-col items-center justify-center gap-3 text-zinc-500 text-xs">
            <RefreshCw class="h-5 w-5 animate-spin text-zinc-400" />
            <span>Scanning declared port bindings...</span>
          </div>
        {:else if filteredAllocs.length === 0}
          <div class="py-12 text-center text-xs text-zinc-500">
            No port allocations match your filter
          </div>
        {:else}
          <table class="w-full text-left border-collapse text-xs">
            <thead>
              <tr class="border-b border-zinc-800/80 bg-zinc-950/60 text-zinc-400 text-[11px] font-mono uppercase tracking-wider">
                <th class="py-2.5 px-4 font-medium">Host Binding</th>
                <th class="py-2.5 px-4 font-medium">Container Port</th>
                <th class="py-2.5 px-4 font-medium">Stack</th>
                <th class="py-2.5 px-4 font-medium">Service</th>
                <th class="py-2.5 px-4 font-medium text-right">Action</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-zinc-800/60 font-mono">
              {#each filteredAllocs as alloc (alloc.port + alloc.stack + alloc.service)}
                <tr class="hover:bg-zinc-800/30 transition-colors">
                  <td class="py-2.5 px-4 font-medium text-white">
                    <span class="text-zinc-400 font-normal">{alloc.host_ip}:</span>{alloc.port}
                    {#if alloc.protocol && alloc.protocol !== 'tcp'}
                      <span class="ml-1 px-1 py-0.2 text-[9px] rounded bg-zinc-800 text-zinc-400 uppercase">
                        {alloc.protocol}
                      </span>
                    {/if}
                  </td>
                  <td class="py-2.5 px-4 text-zinc-400">
                    :{alloc.container_port}
                  </td>
                  <td class="py-2.5 px-4 font-sans text-zinc-300">
                    <span class="px-2 py-0.5 rounded-md bg-zinc-800 text-[11px] border border-zinc-700/50">
                      {alloc.stack}
                    </span>
                  </td>
                  <td class="py-2.5 px-4 font-sans text-zinc-400 text-[11px]">
                    {alloc.service}
                  </td>
                  <td class="py-2.5 px-4 text-right font-sans">
                    {#if onSelectStack}
                      <button
                        onclick={() => {
                          onSelectStack(alloc.stack);
                          onClose();
                        }}
                        class="inline-flex items-center gap-1 text-[11px] text-indigo-400 hover:text-indigo-300 transition-colors cursor-pointer"
                      >
                        <span>Open Stack</span>
                        <ArrowUpRight class="h-3 w-3" />
                      </button>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 border-t border-zinc-800/80 flex items-center justify-between bg-zinc-950/60 text-xs text-zinc-500 shrink-0">
        <span class="text-[11px] text-zinc-500">Host port bindings scanned across all active compose stacks</span>
        <button
          onclick={onClose}
          class="px-3 py-1.5 rounded-md border border-zinc-800 bg-zinc-900 hover:bg-zinc-800 text-zinc-300 hover:text-white text-xs font-medium transition-colors cursor-pointer"
        >
          Close
        </button>
      </div>
    </div>
  </div>
{/if}
