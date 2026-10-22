<script>
  import { fade, scale } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { login, setup } from './api.js';

  let { isSetup = false, onLoginSuccess } = $props();

  let password = $state('');
  let loading = $state(false);
  let error = $state('');

  async function handleSubmit(e) {
    e.preventDefault();
    loading = true;
    error = '';

    try {
      if (isSetup) {
        await setup(password);
      } else {
        await login(password);
      }
      onLoginSuccess();
    } catch (err) {
      error = err.message || 'Authentication failed';
    } finally {
      loading = false;
    }
  }
</script>

<div class="h-dvh flex items-center justify-center p-4 bg-zinc-950" in:fade={{ duration: 150 }}>
  <div
    in:scale={{ duration: 180, start: 0.96, easing: cubicOut }}
    class="w-full max-w-sm rounded-xl border border-zinc-800 bg-zinc-900/90 p-6 shadow-2xl backdrop-blur-xs"
  >
    <div class="flex items-center gap-2.5 mb-2">
      <svg class="h-5 w-5 text-zinc-400" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/>
        <path d="m3.3 7 8.7 5 8.7-5"/>
        <path d="M12 22V12"/>
      </svg>
      <h1 class="text-sm font-semibold text-white">
        {isSetup ? 'Set Admin Password' : 'composebox'}
      </h1>
    </div>

    <p class="text-xs text-zinc-400 mb-5 leading-relaxed">
      {isSetup
        ? 'Create an admin password to secure composebox.'
        : 'Enter your admin password to continue.'}
    </p>

    {#if error}
      <div
        transition:fade={{ duration: 120 }}
        class="mb-4 rounded-md border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-400"
      >
        {error}
      </div>
    {/if}

    <form method="POST" onsubmit={handleSubmit} class="space-y-4">
      <div>
        <label for="passwordInput" class="sr-only">Password</label>
        <div class="relative">
          <input
            type="password"
            id="passwordInput"
            name="password"
            bind:value={password}
            placeholder="Password"
            autocomplete={isSetup ? 'new-password' : 'current-password'}
            required
            class="block w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-base sm:text-sm text-zinc-100 placeholder-zinc-500 shadow-xs focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 transition-colors"
          />
        </div>
      </div>

      <button
        type="submit"
        disabled={loading}
        class="flex w-full justify-center rounded-md bg-blue-600 px-3 py-2 text-xs font-medium text-white shadow-xs hover:bg-blue-500 active:scale-[0.99] focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 focus:ring-offset-zinc-900 disabled:opacity-50 transition-all duration-150"
      >
        {loading ? 'Verifying...' : isSetup ? 'Create Admin Password' : 'Login'}
      </button>
    </form>
  </div>
</div>
