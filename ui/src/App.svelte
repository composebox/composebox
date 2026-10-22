<script>
  import { onMount } from 'svelte';
  import { fade, fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import {
    getAuthStatus,
    getStacks,
    getStack,
    saveStack,
    createStack,
    deleteStack,
    triggerAction,
    logout as apiLogout,
  } from './lib/api.js';
  import Login from './lib/Login.svelte';
  import Sidebar from './lib/Sidebar.svelte';
  import ContainersTable from './lib/ContainersTable.svelte';
  import Editor from './lib/Editor.svelte';
  import Terminal from './lib/Terminal.svelte';
  import ActionConsole from './lib/ActionConsole.svelte';
  import Modal from './lib/Modal.svelte';

  let authState = $state('loading'); // 'loading' | 'setup' | 'login' | 'app'
  let authEnabled = $state(true);
  let version = $state('v0.1.0');
  let stacks = $state([]);
  let currentStack = $state(null);
  let tab = $state('containers'); // 'containers' | 'compose' | 'env' | 'logs'
  let activeAction = $state(null);
  let mobileDrawerOpen = $state(true);

  let consoleOpen = $state(false);
  let consoleAction = $state('');
  let consoleLogs = $state('');
  let consoleStatus = $state('running'); // 'running' | 'success' | 'error'

  let composeContent = $state('');
  let envContent = $state('');
  let originalCompose = $state('');
  let originalEnv = $state('');

  let newStackModalOpen = $state(false);
  let newStackName = $state('');
  let deleteModalOpen = $state(false);

  let toast = $state({ visible: false, message: '', type: '' });
  let toastTimer = null;

  let hasUnsavedChanges = $derived(
    currentStack !== null &&
      (composeContent !== originalCompose || envContent !== originalEnv)
  );

  let isRunning = $derived(
    currentStack !== null &&
      (currentStack.status === 'running' || currentStack.status === 'partial')
  );

  function showToast(message, type = '') {
    if (toastTimer) clearTimeout(toastTimer);
    toast = { visible: true, message, type };
    toastTimer = setTimeout(() => {
      toast.visible = false;
    }, 2500);
  }

  async function checkAuth() {
    try {
      const status = await getAuthStatus();
      if (status.version) {
        version = status.version;
      }
      authEnabled = Boolean(status.auth_enabled);
      if (!status.auth_enabled) {
        authState = 'app';
        loadStacks();
        return;
      }
      if (status.setup_required) {
        authState = 'setup';
        return;
      }
      if (!status.authenticated) {
        authState = 'login';
        return;
      }
      authState = 'app';
      loadStacks();
    } catch (err) {
      showToast('Failed to check auth status', 'error');
      authState = 'login';
    }
  }

  async function loadStacks() {
    try {
      const list = await getStacks();
      stacks = list || [];
      if (currentStack) {
        const updated = stacks.find((s) => s.name === currentStack.name);
        if (updated) {
          currentStack.status = updated.status;
          currentStack.running_count = updated.running_count;
          currentStack.total_services = updated.total_services;
          currentStack.containers = updated.containers || [];
          currentStack.has_env = updated.has_env;
        }
      }
    } catch (err) {
      if (err.status === 401) {
        authState = 'login';
      } else {
        showToast('Failed to load stacks', 'error');
      }
    }
  }

  async function selectStack(name, force = false) {
    if (!force && hasUnsavedChanges && currentStack?.name !== name) {
      if (!window.confirm('You have unsaved changes. Discard them and switch stacks?')) {
        return;
      }
    }
    try {
      currentStack = await getStack(name);
      originalCompose = currentStack.compose_content || '';
      originalEnv = currentStack.env_content || '';
      composeContent = originalCompose;
      envContent = originalEnv;
      mobileDrawerOpen = false;
    } catch (err) {
      showToast(err.message, 'error');
    }
  }

  async function handleAction(action) {
    if (!currentStack || activeAction) return;
    activeAction = action;
    consoleAction = action;
    consoleLogs = `[composebox] Starting action '${action}' for stack '${currentStack.name}'...\n`;
    consoleStatus = 'running';
    consoleOpen = true;

    try {
      await triggerAction(currentStack.name, action, (chunk) => {
        consoleLogs += chunk;
      });
      consoleStatus = 'success';
      showToast(`Action '${action}' completed successfully`, 'success');
      await loadStacks();
      // Reload current stack details to refresh containers and status
      await selectStack(currentStack.name, false);
    } catch (err) {
      consoleStatus = 'error';
      consoleLogs += `\n[composebox:error] ${err.message}\n`;
      showToast(err.message, 'error');
    } finally {
      activeAction = null;
    }
  }

  async function handleSave() {
    if (!currentStack || activeAction) return;
    activeAction = 'save';
    try {
      const res = await saveStack(currentStack.name, composeContent, envContent);
      originalCompose = composeContent;
      originalEnv = envContent;
      showToast(res.message || 'Configuration saved', 'success');
      await loadStacks();
    } catch (err) {
      showToast(err.message, 'error');
    } finally {
      activeAction = null;
    }
  }

  async function handleCreateStack(e) {
    e.preventDefault();
    const name = newStackName.trim();
    if (!name) return;
    try {
      const res = await createStack(name);
      newStackModalOpen = false;
      newStackName = '';
      showToast(res.message, 'success');
      await loadStacks();
      await selectStack(name, true);
    } catch (err) {
      showToast(err.message, 'error');
    }
  }

  async function handleDeleteStack() {
    if (!currentStack) return;
    const name = currentStack.name;
    try {
      const res = await deleteStack(name);
      deleteModalOpen = false;
      showToast(res.message, 'success');
      currentStack = null;
      await loadStacks();
    } catch (err) {
      showToast(err.message, 'error');
    }
  }

  async function handleLogout() {
    await apiLogout();
    currentStack = null;
    stacks = [];
    mobileDrawerOpen = false;
    authState = 'login';
  }

  function handleKeydown(e) {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
      e.preventDefault();
      handleSave();
    }
  }

  onMount(() => {
    checkAuth();
    const poll = setInterval(() => {
      if (authState === 'app') {
        loadStacks();
      }
    }, 6000);
    return () => clearInterval(poll);
  });
</script>

<svelte:window onkeydown={handleKeydown} />

{#if authState === 'loading'}
  <div class="h-dvh flex items-center justify-center bg-zinc-950 text-zinc-500 text-xs">
    Initializing...
  </div>
{:else if authState === 'setup'}
  <Login isSetup={true} onLoginSuccess={() => { authState = 'app'; loadStacks(); }} />
{:else if authState === 'login'}
  <Login isSetup={false} onLoginSuccess={() => { authState = 'app'; loadStacks(); }} />
{:else if authState === 'app'}
  <div class="h-dvh flex overflow-hidden relative" in:fade={{ duration: 150 }}>
    <!-- Sidebar -->
    <Sidebar
      {stacks}
      {currentStack}
      {version}
      {authEnabled}
      mobileOpen={mobileDrawerOpen}
      onSelectStack={(name) => selectStack(name)}
      onOpenNewStack={() => { newStackName = ''; newStackModalOpen = true; }}
      onCloseMobile={() => (mobileDrawerOpen = false)}
      onLogout={handleLogout}
    />

    <!-- Main Workspace -->
    <main class="flex-1 flex flex-col h-full bg-zinc-950 overflow-hidden">
      {#if !currentStack}
        <div class="h-13 px-3.5 border-b border-zinc-800/80 bg-zinc-900/30 flex items-center md:hidden shrink-0">
          <button
            onclick={() => (mobileDrawerOpen = true)}
            aria-label="Open stacks menu"
            class="-ml-1 rounded-md p-1.5 text-zinc-400 hover:bg-zinc-800 hover:text-white active:scale-95 transition-all duration-150"
          >
            <svg class="h-4.5 w-4.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="18" x2="21" y2="18"/>
            </svg>
          </button>
          <span class="ml-2 text-xs font-medium text-zinc-300">Stacks</span>
        </div>
        <div class="flex-1 flex flex-col items-center justify-center text-zinc-500 gap-2 p-4 text-center">
          <svg class="h-8 w-8 text-zinc-700" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/>
            <path d="m3.3 7 8.7 5 8.7-5"/><path d="M12 22V12"/>
          </svg>
          <p class="text-xs">Select a stack to view containers and edit files</p>
        </div>
      {:else}
        <!-- Action bar (responsive 2-row on mobile, 1-row on desktop) -->
        <div class="px-3.5 sm:px-5 py-2.5 sm:py-0 sm:h-13 border-b border-zinc-800/80 bg-zinc-900/30 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2.5 shrink-0">
          <!-- Top row on mobile: Menu button + Stack Name + Status + Mobile Save/Delete -->
          <div class="flex items-center justify-between gap-2 min-w-0">
            <div class="flex items-center gap-2.5 min-w-0">
              <button
                onclick={() => (mobileDrawerOpen = true)}
                aria-label="Open stacks menu"
                class="md:hidden -ml-1 rounded-md p-1.5 text-zinc-400 hover:bg-zinc-800 hover:text-white active:scale-95 transition-all duration-150 shrink-0"
              >
                <svg class="h-4.5 w-4.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="18" x2="21" y2="18"/>
                </svg>
              </button>
              <h2 class="text-sm font-semibold text-white truncate">{currentStack.name}</h2>
              <span
                class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[11px] font-medium shrink-0 transition-colors duration-300 {currentStack.status === 'running' ? 'bg-emerald-500/10 text-emerald-400' : currentStack.status === 'partial' ? 'bg-amber-500/10 text-amber-400' : currentStack.status === 'stopped' || currentStack.status === 'exited' ? 'bg-red-500/10 text-red-400' : 'bg-zinc-800 text-zinc-400'}"
              >
                <span
                  class="h-1.5 w-1.5 rounded-full transition-colors duration-300 {currentStack.status === 'running' ? 'bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.6)]' : currentStack.status === 'partial' ? 'bg-amber-400' : currentStack.status === 'stopped' || currentStack.status === 'exited' ? 'bg-red-400' : 'bg-zinc-400'}"
                ></span>
                <span>
                  {currentStack.status || 'unknown'} ({currentStack.running_count || 0}/{currentStack.total_services || 0})
                </span>
              </span>
            </div>

            <!-- Save & Delete quick buttons on mobile header row -->
            <div class="flex sm:hidden items-center gap-1.5 shrink-0">
              <button
                onclick={handleSave}
                disabled={activeAction !== null}
                title="Save Changes"
                class="relative flex items-center gap-1 rounded-md bg-blue-600 px-2.5 py-1 text-xs font-medium text-white hover:bg-blue-500 active:scale-95 disabled:opacity-50 transition-all duration-150"
              >
                <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"/><polyline points="17 21 17 13 7 13 7 21"/><polyline points="7 3 7 8 15 8"/></svg>
                <span>{activeAction === 'save' ? '...' : 'Save'}</span>
                {#if hasUnsavedChanges}
                  <span class="absolute -top-1 -right-1 h-2 w-2 rounded-full bg-amber-400"></span>
                {/if}
              </button>
              <button
                onclick={() => (deleteModalOpen = true)}
                disabled={activeAction !== null}
                title="Delete Stack"
                class="rounded-md border border-zinc-700/80 bg-zinc-800/60 p-1.5 text-red-400 hover:bg-red-500/10 hover:border-red-500/30 active:scale-95 disabled:opacity-50 transition-all duration-150"
              >
                <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
              </button>
            </div>
          </div>

          <!-- Lifecycle & Desktop Actions -->
          <div class="grid {isRunning ? 'grid-cols-3' : 'grid-cols-2'} sm:flex sm:items-center gap-1.5">
            {#if isRunning}
              <button
                onclick={() => handleAction('update')}
                disabled={activeAction !== null}
                title="Update Stack (Pull latest images & apply changes)"
                class="flex items-center justify-center gap-1.5 rounded-md border border-emerald-500/40 bg-emerald-500/10 px-2.5 py-1.5 sm:py-1 text-xs text-emerald-300 hover:bg-emerald-500/20 active:scale-[0.97] disabled:opacity-50 transition-all duration-150"
              >
                <svg class="h-3.5 w-3.5 shrink-0 {activeAction === 'update' ? 'animate-spin' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12a9 9 0 0 0-9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/><path d="M3 12a9 9 0 0 0 9 9 9.75 9.75 0 0 0 6.74-2.74L21 16"/><path d="M16 16h5v5"/></svg>
                <span>{activeAction === 'update' ? 'Updating' : 'Update'}</span>
              </button>
              <button
                onclick={() => handleAction('restart')}
                disabled={activeAction !== null}
                title="Restart Stack"
                class="flex items-center justify-center gap-1.5 rounded-md border border-zinc-700/80 bg-zinc-800/60 px-2.5 py-1.5 sm:py-1 text-xs text-zinc-200 hover:bg-zinc-700 active:scale-[0.97] disabled:opacity-50 transition-all duration-150"
              >
                <svg class="h-3.5 w-3.5 shrink-0 {activeAction === 'restart' ? 'animate-spin motion-reduce:animate-none' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/></svg>
                <span>{activeAction === 'restart' ? 'Restart' : 'Restart'}</span>
              </button>
              <button
                onclick={() => handleAction('down')}
                disabled={activeAction !== null}
                title="Stop Stack"
                class="flex items-center justify-center gap-1.5 rounded-md border border-zinc-700/80 bg-zinc-800/60 px-2.5 py-1.5 sm:py-1 text-xs text-zinc-200 hover:bg-zinc-700 active:scale-[0.97] disabled:opacity-50 transition-all duration-150"
              >
                <svg class="h-3.5 w-3.5 shrink-0 {activeAction === 'down' ? 'animate-pulse text-red-400' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect width="14" height="14" x="5" y="5" rx="2"/></svg>
                <span>{activeAction === 'down' ? 'Stopping' : 'Stop'}</span>
              </button>
            {:else}
              <button
                onclick={() => handleAction('up')}
                disabled={activeAction !== null}
                title="Start Stack"
                class="flex items-center justify-center gap-1.5 rounded-md border border-emerald-500/40 bg-emerald-500/10 px-2.5 py-1.5 sm:py-1 text-xs text-emerald-300 hover:bg-emerald-500/20 active:scale-[0.97] disabled:opacity-50 transition-all duration-150"
              >
                <svg class="h-3.5 w-3.5 shrink-0 {activeAction === 'up' ? 'animate-pulse text-emerald-400' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"/></svg>
                <span>{activeAction === 'up' ? 'Starting' : 'Start'}</span>
              </button>
              <button
                onclick={() => handleAction('pull')}
                disabled={activeAction !== null}
                title="Pull Images"
                class="flex items-center justify-center gap-1.5 rounded-md border border-zinc-700/80 bg-zinc-800/60 px-2.5 py-1.5 sm:py-1 text-xs text-zinc-200 hover:bg-zinc-700 active:scale-[0.97] disabled:opacity-50 transition-all duration-150"
              >
                <svg class="h-3.5 w-3.5 shrink-0 {activeAction === 'pull' ? 'animate-bounce motion-reduce:animate-none' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" x2="12" y1="15" y2="3"/></svg>
                <span>{activeAction === 'pull' ? 'Pulling' : 'Pull'}</span>
              </button>
            {/if}
            <button
              onclick={handleSave}
              disabled={activeAction !== null}
              title="Save Changes (Ctrl+S)"
              class="hidden sm:flex relative items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1 text-xs font-medium text-white hover:bg-blue-500 active:scale-[0.97] disabled:opacity-50 transition-all duration-150"
            >
              <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"/><polyline points="17 21 17 13 7 13 7 21"/><polyline points="7 3 7 8 15 8"/></svg>
              <span>{activeAction === 'save' ? 'Saving...' : 'Save'}</span>
              {#if hasUnsavedChanges}
                <span class="absolute -top-1 -right-1 h-2 w-2 rounded-full bg-amber-400"></span>
              {/if}
            </button>
            <button
              onclick={() => (deleteModalOpen = true)}
              disabled={activeAction !== null}
              title="Delete Stack"
              class="hidden sm:inline-flex rounded-md border border-zinc-700/80 bg-zinc-800/60 p-1.5 text-red-400 hover:bg-red-500/10 hover:border-red-500/30 active:scale-95 disabled:opacity-50 transition-all duration-150"
            >
              <svg class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
            </button>
          </div>
        </div>

        <!-- Navigation tabs -->
        <nav class="flex items-center justify-between border-b border-zinc-800/80 bg-zinc-900/20 px-3.5 sm:px-5 shrink-0 overflow-x-auto">
          <div class="flex gap-5 sm:gap-6">
            <button
              onclick={() => (tab = 'containers')}
              class="border-b-2 py-2.5 text-xs whitespace-nowrap transition-colors duration-150 {tab === 'containers' ? 'border-blue-500 text-white font-medium' : 'border-transparent text-zinc-400 hover:text-zinc-200'}"
            >
              Containers
            </button>
            <button
              onclick={() => (tab = 'compose')}
              class="border-b-2 py-2.5 text-xs whitespace-nowrap transition-colors duration-150 {tab === 'compose' ? 'border-blue-500 text-white font-medium' : 'border-transparent text-zinc-400 hover:text-zinc-200'}"
            >
              compose.yaml
            </button>
            <button
              onclick={() => (tab = 'env')}
              class="border-b-2 py-2.5 text-xs whitespace-nowrap transition-colors duration-150 {tab === 'env' ? 'border-blue-500 text-white font-medium' : 'border-transparent text-zinc-400 hover:text-zinc-200'}"
            >
              .env
            </button>
            <button
              onclick={() => (tab = 'logs')}
              class="border-b-2 py-2.5 text-xs whitespace-nowrap transition-colors duration-150 {tab === 'logs' ? 'border-blue-500 text-white font-medium' : 'border-transparent text-zinc-400 hover:text-zinc-200'}"
            >
              Logs
            </button>
          </div>

          <button
            onclick={() => (consoleOpen = !consoleOpen)}
            class="flex items-center gap-1.5 px-2.5 py-1 rounded text-xs transition-colors duration-150 {consoleOpen ? 'bg-zinc-800 text-white border border-zinc-700' : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/50'}"
            title="Toggle Action Console Drawer"
          >
            <svg class="h-3 w-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>
            <span>Console</span>
            {#if consoleLogs && !consoleOpen}
              <span class="h-1.5 w-1.5 rounded-full {consoleStatus === 'running' ? 'bg-amber-400 animate-pulse' : consoleStatus === 'error' ? 'bg-red-400' : 'bg-emerald-400'}"></span>
            {/if}
          </button>
        </nav>

        <!-- Content pane with subtle fade transition on tab switch -->
        {#key `${currentStack.name}-${tab}`}
          <div class="flex-1 flex flex-col overflow-hidden" in:fade={{ duration: 120 }}>
            {#if tab === 'containers'}
              <ContainersTable containers={currentStack.containers || []} />
            {:else if tab === 'compose'}
              <Editor bind:value={composeContent} placeholder="services:\n  app:\n    image: nginx\n" />
            {:else if tab === 'env'}
              <Editor bind:value={envContent} placeholder="# KEY=VALUE" />
            {:else if tab === 'logs'}
              <Terminal stackName={currentStack.name} />
            {/if}
          </div>
        {/key}

        <!-- Action Console Drawer -->
        <ActionConsole
          open={consoleOpen}
          action={consoleAction}
          logs={consoleLogs}
          status={consoleStatus}
          onClose={() => (consoleOpen = false)}
          onClear={() => (consoleLogs = '')}
        />
      {/if}
    </main>
  </div>

  <!-- New Stack Modal -->
  <Modal open={newStackModalOpen} onClose={() => (newStackModalOpen = false)}>
    <h3 class="text-sm font-semibold text-white mb-1">Create New Stack</h3>
    <p class="text-xs text-zinc-400 mb-4">Enter a directory name for your new compose stack.</p>
    <form onsubmit={handleCreateStack} class="space-y-4">
      <input
        type="text"
        bind:value={newStackName}
        placeholder="e.g. my-service"
        pattern="[a-zA-Z0-9_\-]+"
        required
        class="block w-full rounded-md border border-zinc-700 bg-zinc-950 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 transition-colors"
      />
      <div class="flex justify-end gap-2">
        <button
          type="button"
          onclick={() => (newStackModalOpen = false)}
          class="rounded-md border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800 active:scale-95 transition-all duration-150"
        >
          Cancel
        </button>
        <button
          type="submit"
          class="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-500 active:scale-95 transition-all duration-150"
        >
          Create Stack
        </button>
      </div>
    </form>
  </Modal>

  <!-- Delete Modal -->
  <Modal open={deleteModalOpen} onClose={() => (deleteModalOpen = false)}>
    <h3 class="text-sm font-semibold text-red-400 mb-1">Delete Stack</h3>
    <p class="text-xs text-zinc-400 mb-4">
      Are you sure you want to permanently delete stack '{currentStack?.name}' from disk?
    </p>
    <div class="flex justify-end gap-2">
      <button
        type="button"
        onclick={() => (deleteModalOpen = false)}
        class="rounded-md border border-zinc-700 px-3 py-1.5 text-xs text-zinc-300 hover:bg-zinc-800 active:scale-95 transition-all duration-150"
      >
        Cancel
      </button>
      <button
        type="button"
        onclick={handleDeleteStack}
        class="rounded-md bg-red-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-500 active:scale-95 transition-all duration-150"
      >
        Delete
      </button>
    </div>
  </Modal>

  <!-- Toast -->
  {#if toast.visible}
    <div
      transition:fly={{ y: 12, duration: 180, easing: cubicOut }}
      class="fixed bottom-4 right-4 left-4 sm:left-auto z-50 rounded-md border px-3.5 py-2 text-xs shadow-lg text-center sm:text-left {toast.type === 'success' ? 'border-emerald-500/30 bg-zinc-900 text-emerald-400' : toast.type === 'error' ? 'border-red-500/30 bg-zinc-900 text-red-400' : 'border-zinc-700 bg-zinc-900 text-zinc-200'}"
    >
      {toast.message}
    </div>
  {/if}
{/if}
