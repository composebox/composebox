<script>
  import { fade } from 'svelte/transition';
  import Logo from './Logo.svelte';

  let {
    stacks = [],
    currentStack = null,
    version = '',
    authEnabled = true,
    mobileOpen = false,
    onSelectStack,
    onOpenNewStack,
    onCloseMobile,
    onLogout,
  } = $props();

  let search = $state('');
  let searchEl = null;

  let filteredStacks = $derived(
    search.trim() === ''
      ? stacks
      : stacks.filter((s) => s.name.toLowerCase().includes(search.toLowerCase()))
  );

  function handleGlobalKeydown(e) {
    if (e.key === '/' && !e.ctrlKey && !e.metaKey && !e.altKey) {
      const tag = document.activeElement?.tagName;
      if (tag !== 'INPUT' && tag !== 'TEXTAREA' && !document.activeElement?.isContentEditable) {
        e.preventDefault();
        searchEl?.focus();
      }
    } else if (e.key === 'Escape' && mobileOpen && onCloseMobile) {
      onCloseMobile();
    }
  }

  function handleSearchKeydown(e) {
    if (e.key === 'Escape') {
      search = '';
      searchEl?.blur();
    }
  }
</script>

<svelte:window onkeydown={handleGlobalKeydown} />

<!-- Mobile backdrop when drawer is open -->
{#if mobileOpen}
  <div
    role="presentation"
    onclick={onCloseMobile}
    transition:fade={{ duration: 150 }}
    class="fixed inset-0 z-30 bg-black/70 backdrop-blur-xs md:hidden"
  ></div>
{/if}

<aside
  class="fixed inset-y-0 left-0 z-40 w-64 border-r border-zinc-800/80 bg-zinc-900 md:bg-zinc-900/60 flex flex-col h-full shrink-0 transition-transform duration-200 ease-out motion-reduce:transition-none md:static md:translate-x-0
    {mobileOpen
      ? 'translate-x-0 shadow-2xl md:shadow-none'
      : '-translate-x-full pointer-events-none md:pointer-events-auto'}"
>
  <!-- Brand row -->
  <div class="h-13 px-4 border-b border-zinc-800/80 flex items-center justify-between shrink-0">
    <div class="flex items-center gap-2 font-semibold text-white">
      <Logo size={16} />
      <span class="text-xs">composebox</span>
    </div>
    <div class="flex items-center gap-1.5">
      <span class="font-mono text-[10px] text-zinc-500">{version}</span>
      {#if authEnabled}
        <button
          onclick={onLogout}
          class="rounded border border-zinc-700/80 bg-zinc-800/50 px-2 py-0.5 text-[10px] text-zinc-300 hover:bg-zinc-700 active:scale-95 transition-all duration-150"
        >
          Logout
        </button>
      {/if}
      {#if mobileOpen}
        <button
          onclick={onCloseMobile}
          aria-label="Close menu"
          class="md:hidden rounded p-1 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200 transition-colors"
        >
          <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M18 6 6 18"/><path d="m6 6 12 12"/>
          </svg>
        </button>
      {/if}
    </div>
  </div>

  <!-- Search & New -->
  <div class="p-2.5 border-b border-zinc-800/80 flex gap-2 items-center shrink-0">
    <div class="relative flex-1">
      <svg class="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-zinc-500 pointer-events-none" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>
      </svg>
      <input
        bind:this={searchEl}
        type="text"
        bind:value={search}
        onkeydown={handleSearchKeydown}
        placeholder="Search stacks... (/)"
        class="w-full rounded-md border border-zinc-800 bg-zinc-950 pl-8 pr-2.5 py-1.5 text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none transition-colors"
      />
    </div>
    <button
      onclick={onOpenNewStack}
      class="flex items-center gap-1 rounded-md border border-zinc-700/80 bg-zinc-800/50 px-2.5 py-1.5 text-xs text-zinc-200 hover:bg-zinc-700 active:scale-95 transition-all duration-150 shrink-0"
    >
      <svg class="h-3 w-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
      <span>New</span>
    </button>
  </div>

  <!-- Stacks list -->
  <div class="flex-1 overflow-y-auto p-1.5 space-y-0.5">
    {#each filteredStacks as stack (stack.name)}
      <div
        role="button"
        tabindex="0"
        onclick={() => onSelectStack(stack.name)}
        onkeydown={(e) => e.key === 'Enter' && onSelectStack(stack.name)}
        class="flex items-center justify-between px-2.5 py-1.5 rounded-md cursor-pointer transition-all duration-150 active:scale-[0.99] {currentStack && currentStack.name === stack.name ? 'bg-zinc-800/90 text-white font-medium shadow-xs' : 'text-zinc-400 hover:bg-zinc-800/40 hover:text-zinc-200'}"
      >
        <div class="flex items-center gap-2 min-w-0">
          <span
            class="h-1.5 w-1.5 rounded-full shrink-0 transition-colors duration-300 {stack.status === 'running' ? 'bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.5)]' : stack.status === 'partial' ? 'bg-amber-500' : stack.status === 'stopped' || stack.status === 'exited' ? 'bg-red-500' : 'bg-zinc-600'}"
          ></span>
          <span class="truncate text-xs">{stack.name}</span>
        </div>
        <span class="font-mono text-[10px] text-zinc-500">
          {stack.running_count || 0}/{stack.total_services || 0}
        </span>
      </div>
    {/each}

    {#if filteredStacks.length === 0}
      <div class="text-center py-6 text-xs text-zinc-500">
        No stacks found
      </div>
    {/if}
  </div>
</aside>
