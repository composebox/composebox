<script>
  import { onMount } from 'svelte';
  import { fade, scale } from 'svelte/transition';
  import {
    HardDrive,
    Trash2,
    RefreshCw,
    ShieldCheck,
    Layers,
    Box,
    Hammer,
    Database,
    X,
    Check,
    CheckCircle2,
    AlertCircle,
    Search,
    ChevronRight,
  } from '@lucide/svelte';
  import { getDiskUsage, pruneSystem, pruneItem } from './api';

  let { isOpen = false, onClose } = $props();

  let loading = $state(true);
  let error = $state(null);
  let report = $state(null);
  let activePrune = $state(null);
  let activeItemPrune = $state(null);
  let confirmingItem = $state(null);
  let confirmTimer = null;
  let pruneLog = $state('');
  let pruneSuccess = $state(false);

  let selectedCategory = $state('Images');
  let itemFilter = $state('');

  function requestConfirm(id) {
    if (confirmTimer) clearTimeout(confirmTimer);
    confirmingItem = id;
    confirmTimer = setTimeout(() => {
      confirmingItem = null;
    }, 4000);
  }

  function cancelConfirm() {
    if (confirmTimer) clearTimeout(confirmTimer);
    confirmingItem = null;
  }

  async function loadData() {
    loading = true;
    error = null;
    try {
      report = await getDiskUsage();
    } catch (err) {
      error = err.message || 'Failed to load disk usage';
    } finally {
      loading = false;
    }
  }

  async function handlePrune(target) {
    if (activePrune) return;
    cancelConfirm();
    activePrune = target;
    pruneLog = '';
    pruneSuccess = false;
    error = null;

    try {
      await pruneSystem(target, (chunk, full) => {
        pruneLog = full;
      });
      pruneSuccess = true;
      const updated = await getDiskUsage();
      report = updated;
    } catch (err) {
      error = err.message || 'Prune operation failed';
    } finally {
      activePrune = null;
    }
  }

  async function executePruneItem(type, id) {
    cancelConfirm();
    if (activePrune || activeItemPrune) return;

    activeItemPrune = id;
    pruneLog = '';
    pruneSuccess = false;
    error = null;

    try {
      await pruneItem(type, id, (chunk, full) => {
        pruneLog = full;
      });
      pruneSuccess = true;
      const updated = await getDiskUsage();
      report = updated;
    } catch (err) {
      error = err.message || `Failed to remove ${type}`;
    } finally {
      activeItemPrune = null;
    }
  }

  $effect(() => {
    if (isOpen) {
      loadData();
      pruneLog = '';
      pruneSuccess = false;
      itemFilter = '';
      cancelConfirm();
    }
  });

  function handleKeydown(e) {
    if (e.key === 'Escape' && isOpen && !activePrune) {
      onClose();
    }
  }

  let filteredImages = $derived(
    !report?.images
      ? []
      : report.images.filter((img) => {
          const q = itemFilter.toLowerCase();
          return (
            img.repository.toLowerCase().includes(q) ||
            img.tag.toLowerCase().includes(q) ||
            img.id.toLowerCase().includes(q)
          );
        })
  );

  let filteredContainers = $derived(
    !report?.containers
      ? []
      : report.containers.filter((c) => {
          const q = itemFilter.toLowerCase();
          return (
            c.names.toLowerCase().includes(q) ||
            c.image.toLowerCase().includes(q) ||
            c.id.toLowerCase().includes(q)
          );
        })
  );

  let filteredBuildCache = $derived(
    !report?.build_cache
      ? []
      : report.build_cache.filter((b) => {
          const q = itemFilter.toLowerCase();
          return b.description.toLowerCase().includes(q) || b.id.toLowerCase().includes(q);
        })
  );

  let filteredVolumes = $derived(
    !report?.volumes
      ? []
      : report.volumes.filter((v) => v.name.toLowerCase().includes(itemFilter.toLowerCase()))
  );
</script>

<svelte:window onkeydown={handleKeydown} />

{#if isOpen}
  <!-- Backdrop -->
  <div
    role="presentation"
    onclick={() => !activePrune && onClose()}
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
      class="relative w-full max-w-4xl max-h-[92vh] overflow-hidden rounded-xl border border-zinc-800 bg-zinc-900 shadow-2xl flex flex-col text-zinc-100"
    >
      <!-- Header -->
      <div class="px-5 py-4 border-b border-zinc-800 flex items-center justify-between shrink-0 bg-zinc-900/90">
        <div class="flex items-center gap-2.5">
          <div class="p-2 rounded-lg bg-blue-500/10 border border-blue-500/20 text-blue-400">
            <HardDrive class="h-4 w-4" />
          </div>
          <div>
            <h2 class="text-sm font-semibold text-zinc-100">Docker Disk Hygiene & System Storage</h2>
            <p class="text-xs text-zinc-400">Inspect container disk allocations and reclaim storage safely</p>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <button
            onclick={loadData}
            disabled={loading || activePrune !== null}
            title="Refresh metrics"
            class="p-1.5 rounded-md text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800 disabled:opacity-40 transition-colors cursor-pointer"
          >
            <RefreshCw class="h-4 w-4 {loading ? 'animate-spin' : ''}" />
          </button>
          <button
            onclick={() => !activePrune && onClose()}
            disabled={activePrune !== null}
            class="p-1.5 rounded-md text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800 disabled:opacity-40 transition-colors cursor-pointer"
          >
            <X class="h-4 w-4" />
          </button>
        </div>
      </div>

      <!-- Scrollable Content -->
      <div class="flex-1 overflow-y-auto p-5 space-y-5">
        {#if error}
          <div class="p-3 rounded-lg bg-red-500/10 border border-red-500/30 flex items-start gap-2.5 text-xs text-red-400">
            <AlertCircle class="h-4 w-4 shrink-0 mt-0.5" />
            <div class="flex-1">{error}</div>
          </div>
        {/if}

        {#if loading && !report}
          <div class="py-16 flex flex-col items-center justify-center gap-3 text-zinc-500 text-xs">
            <RefreshCw class="h-5 w-5 animate-spin text-zinc-400" />
            <span>Querying Docker storage allocation...</span>
          </div>
        {:else if report}
          <!-- Reclaimable Space Banner -->
          <div class="p-4 rounded-lg bg-zinc-950/60 border border-zinc-800/80 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
            <div>
              <div class="text-[11px] font-mono text-zinc-500 uppercase tracking-wider">Total Reclaimable Space</div>
              <div class="text-2xl font-bold font-mono text-emerald-400 mt-0.5">
                {report.total_reclaimable || '0 B'}
              </div>
              <div class="text-xs text-zinc-400 mt-0.5">
                Total Allocated: <span class="font-mono text-zinc-300">{report.total_size || '0 B'}</span>
              </div>
            </div>

            <button
              onclick={() => handlePrune('all_safe')}
              disabled={activePrune !== null || report.total_reclaimable === '0 B'}
              class="inline-flex items-center justify-center gap-2 px-3.5 py-2 text-xs font-medium rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white shadow-xs active:scale-[0.98] disabled:opacity-40 disabled:hover:bg-emerald-600 transition-all cursor-pointer disabled:cursor-not-allowed shrink-0"
            >
              <Trash2 class="h-3.5 w-3.5" />
              <span>{activePrune === 'all_safe' ? 'Reclaiming...' : 'Prune All Safe Storage'}</span>
            </button>
          </div>

          <!-- Category Cards Grid (Click to Inspect) -->
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            {#each report.categories as cat}
              <div
                role="button"
                tabindex="0"
                onclick={() => (selectedCategory = cat.type)}
                onkeydown={(e) => e.key === 'Enter' && (selectedCategory = cat.type)}
                class="p-3.5 rounded-lg border transition-all cursor-pointer flex flex-col justify-between {selectedCategory === cat.type ? 'bg-zinc-950/90 border-blue-500/50 shadow-xs' : 'bg-zinc-950/40 border-zinc-800 hover:border-zinc-700'}"
              >
                <div>
                  <div class="flex items-center justify-between">
                    <div class="flex items-center gap-2">
                      {#if cat.type === 'Images'}
                        <Layers class="h-4 w-4 text-blue-400" />
                      {:else if cat.type === 'Build Cache'}
                        <Hammer class="h-4 w-4 text-amber-400" />
                      {:else if cat.type === 'Containers'}
                        <Box class="h-4 w-4 text-purple-400" />
                      {:else}
                        <Database class="h-4 w-4 text-emerald-400" />
                      {/if}
                      <span class="text-xs font-semibold text-zinc-200">{cat.type}</span>
                    </div>
                    <span class="text-[11px] font-mono text-zinc-400">{cat.size}</span>
                  </div>

                  <div class="mt-2.5 flex items-baseline justify-between text-xs">
                    <span class="text-zinc-500">Reclaimable:</span>
                    <span class="font-mono font-medium text-emerald-400">{cat.reclaimable}</span>
                  </div>
                  <div class="text-[11px] text-zinc-500 mt-0.5">
                    {cat.total_count} total ({cat.active_count} active)
                  </div>
                </div>

                <div class="mt-3.5 pt-3 border-t border-zinc-800/80 flex items-center justify-between">
                  <span class="text-[11px] text-zinc-500 hover:text-zinc-300 flex items-center gap-1">
                    {selectedCategory === cat.type ? 'Viewing list ↓' : 'Click to inspect'}
                  </span>

                  {#if cat.type === 'Images'}
                    <button
                      onclick={(e) => { e.stopPropagation(); handlePrune('images'); }}
                      disabled={activePrune !== null}
                      title="Safely removes untagged (<none>) build artifacts while preserving tagged images"
                      class="px-2.5 py-1 text-[11px] font-medium rounded-md border border-zinc-700 bg-zinc-800 hover:bg-zinc-700 text-zinc-200 hover:text-white disabled:opacity-40 transition-colors cursor-pointer"
                    >
                      {activePrune === 'images' ? 'Pruning...' : 'Prune Dangling'}
                    </button>
                  {:else if cat.type === 'Build Cache'}
                    <button
                      onclick={(e) => { e.stopPropagation(); handlePrune('build_cache'); }}
                      disabled={activePrune !== null}
                      class="px-2.5 py-1 text-[11px] font-medium rounded-md border border-zinc-700 bg-zinc-800 hover:bg-zinc-700 text-zinc-200 hover:text-white disabled:opacity-40 transition-colors cursor-pointer"
                    >
                      {activePrune === 'build_cache' ? 'Clearing...' : 'Clear Build Cache'}
                    </button>
                  {:else if cat.type === 'Containers'}
                    <button
                      onclick={(e) => { e.stopPropagation(); handlePrune('containers'); }}
                      disabled={activePrune !== null}
                      class="px-2.5 py-1 text-[11px] font-medium rounded-md border border-zinc-700 bg-zinc-800 hover:bg-zinc-700 text-zinc-200 hover:text-white disabled:opacity-40 transition-colors cursor-pointer"
                    >
                      {activePrune === 'containers' ? 'Pruning...' : 'Prune Stopped'}
                    </button>
                  {:else}
                    <div class="flex items-center gap-1.5 text-[11px] text-zinc-400">
                      <ShieldCheck class="h-3.5 w-3.5 text-emerald-400" />
                      <span>Protected</span>
                    </div>
                  {/if}
                </div>
              </div>
            {/each}
          </div>

          <!-- Item Breakdown Section for Selected Category -->
          <div class="rounded-xl border border-zinc-800 bg-zinc-950 overflow-hidden flex flex-col">
            <!-- Table Header / Search Bar -->
            <div class="px-4 py-3 border-b border-zinc-800 flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 bg-zinc-950">
              <div class="flex items-center gap-2">
                <span class="text-xs font-semibold text-zinc-200">
                  Detailed {selectedCategory} Inventory
                </span>
              </div>
              <div class="relative w-full sm:w-64">
                <Search class="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-zinc-500" />
                <input
                  type="text"
                  bind:value={itemFilter}
                  placeholder={`Filter ${selectedCategory.toLowerCase()}...`}
                  class="w-full pl-8 pr-3 py-1 text-xs rounded-md bg-zinc-900 border border-zinc-800 text-zinc-200 placeholder-zinc-500 focus:outline-none focus:border-blue-500/50"
                />
              </div>
            </div>

            <!-- Table Content -->
            <div class="overflow-x-auto max-h-64 overflow-y-auto text-xs">
              {#if selectedCategory === 'Images'}
                {#if filteredImages.length === 0}
                  <div class="p-8 text-center text-zinc-500">No images match your filter</div>
                {:else}
                  <table class="w-full text-left border-collapse whitespace-nowrap">
                    <thead>
                      <tr class="border-b border-zinc-800 bg-zinc-950 text-[11px] font-mono text-zinc-400 uppercase">
                        <th class="py-2.5 px-3 font-medium">Repository : Tag</th>
                        <th class="py-2.5 px-3 font-medium">Image ID</th>
                        <th class="py-2.5 px-3 font-medium">Size</th>
                        <th class="py-2.5 px-3 font-medium">Created</th>
                        <th class="py-2.5 px-3 font-medium">Status</th>
                        <th class="py-2.5 px-3 text-right font-medium w-32 shrink-0 sticky right-0 bg-zinc-950">Action</th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-zinc-800/60 font-mono text-[11px]">
                      {#each filteredImages as img (img.id + img.repository + img.tag)}
                        <tr class="group h-9 hover:bg-zinc-900 transition-colors">
                          <td class="py-1 px-3 text-zinc-200 font-sans max-w-xs truncate">
                            {#if img.isDangling}
                              <span class="text-amber-400 font-mono">&lt;none&gt;:&lt;none&gt;</span>
                            {:else}
                              <span class="font-medium text-white">{img.repository}</span>:<span class="text-zinc-400">{img.tag}</span>
                            {/if}
                          </td>
                          <td class="py-1 px-3 text-zinc-500 font-mono">{img.id}</td>
                          <td class="py-1 px-3 text-zinc-300 font-mono">{img.size}</td>
                          <td class="py-1 px-3 text-zinc-500 font-sans">{img.created_since}</td>
                          <td class="py-1 px-3">
                            {#if img.isDangling}
                              <span class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] whitespace-nowrap bg-amber-500/10 text-amber-400 border border-amber-500/30" title="Untagged build leftover; deleted by Prune Dangling">
                                Dangling
                              </span>
                            {:else if img.containers === '0' || !img.containers}
                              <span class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] whitespace-nowrap bg-zinc-800 text-zinc-400 border border-zinc-700/80" title="Tagged image with 0 running containers; preserved by safe prune">
                                Inactive (Tagged)
                              </span>
                            {:else}
                              <span class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] whitespace-nowrap bg-emerald-500/10 text-emerald-400 border border-emerald-500/30" title="{img.containers} container(s) using this image">
                                In Use ({img.containers})
                              </span>
                            {/if}
                          </td>
                          <td class="py-1 px-3 text-right font-sans w-32 shrink-0 sticky right-0 bg-zinc-950 group-hover:bg-zinc-900 transition-colors">
                            <div class="h-6 flex items-center justify-end w-full">
                              {#if activeItemPrune === (img.isDangling ? img.id : `${img.repository}:${img.tag}`)}
                                <div class="h-6 inline-flex items-center gap-1 text-[10px] text-zinc-400 justify-end">
                                  <RefreshCw class="h-3 w-3 animate-spin text-red-400" />
                                  <span>Pruning...</span>
                                </div>
                              {:else if confirmingItem === (img.isDangling ? img.id : `${img.repository}:${img.tag}`)}
                                <div class="h-6 inline-flex items-center gap-1 justify-end animate-in fade-in duration-100">
                                  <button
                                    onclick={() => executePruneItem('image', img.isDangling ? img.id : `${img.repository}:${img.tag}`)}
                                    title="Confirm deletion"
                                    class="h-6 inline-flex items-center gap-1 px-2 rounded text-[10px] font-medium bg-red-600 hover:bg-red-500 text-white shadow-2xs active:scale-95 transition-all cursor-pointer whitespace-nowrap"
                                  >
                                    <Check class="h-3 w-3" />
                                    <span>Confirm</span>
                                  </button>
                                  <button
                                    onclick={cancelConfirm}
                                    title="Cancel"
                                    class="h-6 w-6 inline-flex items-center justify-center rounded text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors cursor-pointer"
                                  >
                                    <X class="h-3 w-3" />
                                  </button>
                                </div>
                              {:else}
                                <button
                                  onclick={() => requestConfirm(img.isDangling ? img.id : `${img.repository}:${img.tag}`)}
                                  disabled={activePrune !== null || activeItemPrune !== null}
                                  title={img.isDangling ? "Remove dangling image (docker rmi)" : `Remove image '${img.repository}:${img.tag}' (docker rmi)`}
                                  class="h-6 inline-flex items-center gap-1 px-2 rounded text-[10px] text-zinc-400 hover:text-red-400 hover:bg-red-500/10 border border-zinc-800 hover:border-red-500/30 disabled:opacity-30 transition-colors cursor-pointer whitespace-nowrap"
                                >
                                  <Trash2 class="h-3 w-3" />
                                  <span>Prune</span>
                                </button>
                              {/if}
                            </div>
                          </td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                {/if}

              {:else if selectedCategory === 'Containers'}
                {#if filteredContainers.length === 0}
                  <div class="p-8 text-center text-zinc-500">No containers match your filter</div>
                {:else}
                  <table class="w-full text-left border-collapse whitespace-nowrap">
                    <thead>
                      <tr class="border-b border-zinc-800 bg-zinc-950 text-[11px] font-mono text-zinc-400 uppercase">
                        <th class="py-2.5 px-3 font-medium">Container Name</th>
                        <th class="py-2.5 px-3 font-medium">Image</th>
                        <th class="py-2.5 px-3 font-medium">Status</th>
                        <th class="py-2.5 px-3 font-medium">Size</th>
                        <th class="py-2.5 px-3 text-right font-medium w-32 shrink-0 sticky right-0 bg-zinc-950">Action</th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-zinc-800/60 font-mono text-[11px]">
                      {#each filteredContainers as ctr (ctr.id + ctr.names)}
                        <tr class="group h-9 hover:bg-zinc-900 transition-colors">
                          <td class="py-1 px-3 text-white font-sans font-medium max-w-xs truncate">{ctr.names}</td>
                          <td class="py-1 px-3 text-zinc-400 max-w-xs truncate">{ctr.image}</td>
                          <td class="py-1 px-3 font-sans text-zinc-300">
                            {#if ctr.status.toLowerCase().startsWith('up')}
                              <span class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] whitespace-nowrap bg-emerald-500/10 text-emerald-400 border border-emerald-500/30">
                                {ctr.status}
                              </span>
                            {:else}
                              <span class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] whitespace-nowrap bg-zinc-800 text-zinc-400 border border-zinc-700/80">
                                {ctr.status}
                              </span>
                            {/if}
                          </td>
                          <td class="py-1 px-3 text-zinc-400">{ctr.size}</td>
                          <td class="py-1 px-3 text-right font-sans w-32 shrink-0 sticky right-0 bg-zinc-950 group-hover:bg-zinc-900 transition-colors">
                            <div class="h-6 flex items-center justify-end w-full">
                              {#if ctr.status.toLowerCase().startsWith('up')}
                                <span class="h-6 inline-flex items-center text-[10px] text-zinc-500 italic select-none" title="Container is running. Stop it before removing.">Active</span>
                              {:else if activeItemPrune === ctr.id}
                                <div class="h-6 inline-flex items-center gap-1 text-[10px] text-zinc-400 justify-end">
                                  <RefreshCw class="h-3 w-3 animate-spin text-red-400" />
                                  <span>Removing...</span>
                                </div>
                              {:else if confirmingItem === ctr.id}
                                <div class="h-6 inline-flex items-center gap-1 justify-end animate-in fade-in duration-100">
                                  <button
                                    onclick={() => executePruneItem('container', ctr.id)}
                                    title="Confirm removal"
                                    class="h-6 inline-flex items-center gap-1 px-2 rounded text-[10px] font-medium bg-red-600 hover:bg-red-500 text-white shadow-2xs active:scale-95 transition-all cursor-pointer whitespace-nowrap"
                                  >
                                    <Check class="h-3 w-3" />
                                    <span>Confirm</span>
                                  </button>
                                  <button
                                    onclick={cancelConfirm}
                                    title="Cancel"
                                    class="h-6 w-6 inline-flex items-center justify-center rounded text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors cursor-pointer"
                                  >
                                    <X class="h-3 w-3" />
                                  </button>
                                </div>
                              {:else}
                                <button
                                  onclick={() => requestConfirm(ctr.id)}
                                  disabled={activePrune !== null || activeItemPrune !== null}
                                  title="Remove container (docker rm)"
                                  class="h-6 inline-flex items-center gap-1 px-2 rounded text-[10px] text-zinc-400 hover:text-red-400 hover:bg-red-500/10 border border-zinc-800 hover:border-red-500/30 disabled:opacity-30 transition-colors cursor-pointer whitespace-nowrap"
                                >
                                  <Trash2 class="h-3 w-3" />
                                  <span>Remove</span>
                                </button>
                              {/if}
                            </div>
                          </td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                {/if}

              {:else if selectedCategory === 'Build Cache'}
                {#if filteredBuildCache.length === 0}
                  <div class="p-8 text-center text-zinc-500">No build cache steps found</div>
                {:else}
                  <table class="w-full text-left border-collapse whitespace-nowrap">
                    <thead>
                      <tr class="border-b border-zinc-800 bg-zinc-950 text-[11px] font-mono text-zinc-400 uppercase">
                        <th class="py-2.5 px-3 font-medium">Cache Step Description</th>
                        <th class="py-2.5 px-3 font-medium">Cache Type</th>
                        <th class="py-2.5 px-3 font-medium">Size</th>
                        <th class="py-2.5 px-3 text-right font-medium w-32 shrink-0 sticky right-0 bg-zinc-950">Action</th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-zinc-800/60 font-mono text-[11px]">
                      {#each filteredBuildCache as b (b.id)}
                        <tr class="group h-9 hover:bg-zinc-900 transition-colors">
                          <td class="py-1 px-3 text-zinc-200 font-sans truncate max-w-sm whitespace-nowrap" title={b.description}>{b.description}</td>
                          <td class="py-1 px-3 text-zinc-500">{b.cache_type}</td>
                          <td class="py-1 px-3 text-zinc-300">{b.size}</td>
                          <td class="py-1 px-3 text-right font-sans w-32 shrink-0 sticky right-0 bg-zinc-950 group-hover:bg-zinc-900 transition-colors">
                            <div class="h-6 flex items-center justify-end w-full">
                              {#if activeItemPrune === b.id}
                                <div class="h-6 inline-flex items-center gap-1 text-[10px] text-zinc-400 justify-end">
                                  <RefreshCw class="h-3 w-3 animate-spin text-red-400" />
                                  <span>Clearing...</span>
                                </div>
                              {:else if confirmingItem === b.id}
                                <div class="h-6 inline-flex items-center gap-1 justify-end animate-in fade-in duration-100">
                                  <button
                                    onclick={() => executePruneItem('build_cache', b.id)}
                                    title="Confirm clearing cache"
                                    class="h-6 inline-flex items-center gap-1 px-2 rounded text-[10px] font-medium bg-red-600 hover:bg-red-500 text-white shadow-2xs active:scale-95 transition-all cursor-pointer whitespace-nowrap"
                                  >
                                    <Check class="h-3 w-3" />
                                    <span>Confirm</span>
                                  </button>
                                  <button
                                    onclick={cancelConfirm}
                                    title="Cancel"
                                    class="h-6 w-6 inline-flex items-center justify-center rounded text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors cursor-pointer"
                                  >
                                    <X class="h-3 w-3" />
                                  </button>
                                </div>
                              {:else}
                                <button
                                  onclick={() => requestConfirm(b.id)}
                                  disabled={activePrune !== null || activeItemPrune !== null}
                                  title="Clear cache entry"
                                  class="h-6 inline-flex items-center gap-1 px-2 rounded text-[10px] text-zinc-400 hover:text-red-400 hover:bg-red-500/10 border border-zinc-800 hover:border-red-500/30 disabled:opacity-30 transition-colors cursor-pointer whitespace-nowrap"
                                >
                                  <Trash2 class="h-3 w-3" />
                                  <span>Clear</span>
                                </button>
                              {/if}
                            </div>
                          </td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                {/if}

              {:else if selectedCategory === 'Local Volumes'}
                {#if filteredVolumes.length === 0}
                  <div class="p-8 text-center text-zinc-500">No named volumes found</div>
                {:else}
                  <table class="w-full text-left border-collapse whitespace-nowrap">
                    <thead>
                      <tr class="border-b border-zinc-800 bg-zinc-950 text-[11px] font-mono text-zinc-400 uppercase">
                        <th class="py-2.5 px-3 font-medium">Volume Name</th>
                        <th class="py-2.5 px-3 font-medium">Attached Links</th>
                        <th class="py-2.5 px-3 font-medium">Size</th>
                        <th class="py-2.5 px-3 text-right font-medium w-32 shrink-0 sticky right-0 bg-zinc-950">Action</th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-zinc-800/60 font-mono text-[11px]">
                      {#each filteredVolumes as vol (vol.name)}
                        <tr class="group h-9 hover:bg-zinc-900 transition-colors">
                          <td class="py-1 px-3 text-white font-sans font-medium max-w-xs truncate">{vol.name}</td>
                          <td class="py-1 px-3 text-zinc-500">
                            {#if vol.links !== '0'}
                              <span class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] whitespace-nowrap bg-emerald-500/10 text-emerald-400 border border-emerald-500/30" title="Volume is attached to active/exited container(s)">
                                In Use ({vol.links})
                              </span>
                            {:else}
                              <span class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] whitespace-nowrap bg-zinc-800 text-zinc-400 border border-zinc-700/80" title="Unattached volume">
                                Unattached (0)
                              </span>
                            {/if}
                          </td>
                          <td class="py-1 px-3 text-zinc-300">{vol.size}</td>
                          <td class="py-1 px-3 text-right font-sans w-32 shrink-0 sticky right-0 bg-zinc-950 group-hover:bg-zinc-900 transition-colors">
                            <div class="h-6 flex items-center justify-end w-full">
                              {#if vol.links !== '0'}
                                <span class="h-6 inline-flex items-center text-[10px] text-zinc-500 italic select-none" title="Volume is attached to active/exited container(s)">In Use</span>
                              {:else if activeItemPrune === vol.name}
                                <div class="h-6 inline-flex items-center gap-1 text-[10px] text-zinc-400 justify-end">
                                  <RefreshCw class="h-3 w-3 animate-spin text-red-400" />
                                  <span>Deleting...</span>
                                </div>
                              {:else if confirmingItem === vol.name}
                                <div class="h-6 inline-flex items-center gap-1 justify-end animate-in fade-in duration-100">
                                  <button
                                    onclick={() => executePruneItem('volume', vol.name)}
                                    title="Confirm volume deletion"
                                    class="h-6 inline-flex items-center gap-1 px-2 rounded text-[10px] font-medium bg-red-600 hover:bg-red-500 text-white shadow-2xs active:scale-95 transition-all cursor-pointer whitespace-nowrap"
                                  >
                                    <Check class="h-3 w-3" />
                                    <span>Confirm</span>
                                  </button>
                                  <button
                                    onclick={cancelConfirm}
                                    title="Cancel"
                                    class="h-6 w-6 inline-flex items-center justify-center rounded text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors cursor-pointer"
                                  >
                                    <X class="h-3 w-3" />
                                  </button>
                                </div>
                              {:else}
                                <button
                                  onclick={() => requestConfirm(vol.name)}
                                  disabled={activePrune !== null || activeItemPrune !== null}
                                  title="Delete unattached volume (docker volume rm)"
                                  class="h-6 inline-flex items-center gap-1 px-2 rounded text-[10px] text-zinc-400 hover:text-red-400 hover:bg-red-500/10 border border-zinc-800 hover:border-red-500/30 disabled:opacity-30 transition-colors cursor-pointer whitespace-nowrap"
                                >
                                  <Trash2 class="h-3 w-3" />
                                  <span>Delete</span>
                                </button>
                              {/if}
                            </div>
                          </td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                {/if}
              {/if}
            </div>
          </div>

          <!-- Console Terminal Output for Prune -->
          {#if pruneLog || activePrune || activeItemPrune}
            <div class="rounded-lg border border-zinc-800 bg-zinc-950 p-3 font-mono text-[11px] leading-relaxed">
              <div class="flex items-center justify-between pb-2 mb-2 border-b border-zinc-800/80 text-zinc-500">
                <span class="flex items-center gap-1.5">
                  {#if activePrune || activeItemPrune}
                    <RefreshCw class="h-3 w-3 animate-spin text-blue-400" />
                    <span class="text-blue-400 font-sans text-xs">Operation in progress...</span>
                  {:else if pruneSuccess}
                    <CheckCircle2 class="h-3.5 w-3.5 text-emerald-400" />
                    <span class="text-emerald-400 font-sans text-xs">Operation finished</span>
                  {/if}
                </span>
                <span class="text-[10px]">Docker Prune Console</span>
              </div>
              <pre class="overflow-x-auto text-zinc-300 max-h-40 whitespace-pre-wrap">{pruneLog || 'Initializing prune process...'}</pre>
            </div>
          {/if}
        {/if}
      </div>

      <!-- Footer (Cleaned up, no docker.sock clutter) -->
      <div class="px-5 py-3 border-t border-zinc-800/80 flex items-center justify-between bg-zinc-950/60 text-xs text-zinc-500">
        <span>{report?.images?.length || 0} images · {report?.containers?.length || 0} containers · {report?.build_cache?.length || 0} build cache steps</span>
        <button
          onclick={() => !activePrune && onClose()}
          disabled={activePrune !== null}
          class="px-3.5 py-1.5 rounded-md border border-zinc-800 bg-zinc-900 hover:bg-zinc-800 text-zinc-300 hover:text-white text-xs font-medium transition-colors cursor-pointer"
        >
          Close
        </button>
      </div>
    </div>
  </div>
{/if}
