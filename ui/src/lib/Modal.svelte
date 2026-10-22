<script>
  import { fade, scale } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';

  let { open = false, onClose, maxWidth = 'max-w-sm', children } = $props();

  function handleKeydown(e) {
    if (e.key === 'Escape' && open) {
      onClose();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if open}
  <div
    role="dialog"
    aria-modal="true"
    tabindex="-1"
    transition:fade={{ duration: 150 }}
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-xs p-4"
    onclick={(e) => e.target === e.currentTarget && onClose()}
    onkeydown={(e) => e.key === 'Escape' && onClose()}
  >
    <div
      transition:scale={{ duration: 160, start: 0.95, easing: cubicOut }}
      class="w-full {maxWidth} rounded-xl border border-zinc-800 bg-zinc-900 p-5 shadow-2xl"
    >
      {@render children()}
    </div>
  </div>
{/if}
