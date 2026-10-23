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
  import Logo from './lib/Logo.svelte';
  import { DropdownMenu } from 'bits-ui';
  import {
    Save,
    Rocket,
    Play,
    RotateCw,
    RefreshCw,
    Square,
    Trash2,
    ChevronDown,
    Terminal as TerminalIcon,
    Menu,
    Download,
    X,
  } from '@lucide/svelte';

  let authState = $state('loading'); // 'loading' | 'setup' | 'login' | 'app'
  let authEnabled = $state(true);
  let version = $state('');
  let stacks = $state([]);
  let currentStack = $state(null);
  let tab = $state('containers'); // 'containers' | 'compose' | 'env' | 'logs'
  let activeAction = $state(null);
  let mobileDrawerOpen = $state(true);

  let consoleOpen = $state(false);
  let consoleAction = $state('');
  let consoleLogs = $state('');
  let consoleStatus = $state('idle'); // 'idle' | 'running' | 'success' | 'error'

  let composeContent = $state('');
  let envContent = $state('');
  let originalCompose = $state('');
  let originalEnv = $state('');

  let newStackModalOpen = $state(false);
  let newStackName = $state('');
  let deleteModalOpen = $state(false);
  let stopModalOpen = $state(false);
  let copiedRecovery = $state(false);

  function copyRecoveryCommand() {
    const cmd = `cd ${currentStack?.path || '/home/ubuntu/services/composebox'} && docker compose up -d`;
    navigator.clipboard?.writeText(cmd);
    copiedRecovery = true;
    setTimeout(() => {
      copiedRecovery = false;
    }, 2000);
  }

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

  async function waitForServerOnline(timeoutMs = 25000) {
    const start = Date.now();
    await new Promise((r) => setTimeout(r, 1200));
    while (Date.now() - start < timeoutMs) {
      try {
        const res = await fetch('/api/health', {
          cache: 'no-store',
          headers: { 'X-Composebox-Client': 'true' },
        });
        if (res.ok) {
          return true;
        }
      } catch {
        // Still rebooting
      }
      await new Promise((r) => setTimeout(r, 1000));
    }
    return false;
  }

  async function handleAction(action) {
    if (!currentStack || activeAction) return;
    activeAction = action;
    consoleAction = action;
    consoleLogs = `[composebox] Starting action '${action}' for stack '${currentStack.name}'...\n`;
    consoleStatus = 'running';
    consoleOpen = true;

    const isSelfAction =
      currentStack.name?.toLowerCase() === 'composebox' &&
      (action === 'restart' || action === 'update' || action === 'up');

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
      if (isSelfAction) {
        consoleLogs += `\n[composebox] Server connection dropped while restarting Composebox container.\n`;
        consoleLogs += `[composebox] Reconnecting to server...\n`;
        const online = await waitForServerOnline();
        if (online) {
          consoleStatus = 'success';
          consoleLogs += `[composebox] Connection restored! Composebox is back online.\n`;
          showToast(`Action '${action}' completed successfully`, 'success');
          await loadStacks();
          await selectStack(currentStack.name, false);
          return;
        }
      }
      consoleStatus = 'error';
      consoleLogs += `\n[composebox:error] ${err.message}\n`;
      showToast(err.message, 'error');
    } finally {
      activeAction = null;
    }
  }

  function handleClearConsole() {
    consoleLogs = '';
    consoleStatus = 'idle';
    consoleAction = '';
  }

  async function handleSave(deploy = false) {
    if (!currentStack || activeAction) return;
    if (!deploy && !hasUnsavedChanges) return;

    if (hasUnsavedChanges) {
      activeAction = 'save';
      try {
        const res = await saveStack(currentStack.name, composeContent, envContent);
        originalCompose = composeContent;
        originalEnv = envContent;
        if (!deploy) {
          showToast(res.message || 'Configuration saved to disk', 'success');
          await loadStacks();
        }
      } catch (err) {
        showToast(err.message, 'error');
        activeAction = null;
        return;
      }
      activeAction = null;
    }

    if (deploy) {
      await handleAction('up');
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
      if (e.shiftKey) {
        handleSave(true);
      } else if (hasUnsavedChanges) {
        handleSave(false);
      }
    }
  }

  onMount(() => {
    checkAuth();

    const handleVisibilityChange = () => {
      if (!document.hidden && authState === 'app') {
        loadStacks();
      }
    };
    document.addEventListener('visibilitychange', handleVisibilityChange);

    const poll = setInterval(() => {
      if (authState === 'app' && !document.hidden) {
        loadStacks();
      }
    }, 6000);

    return () => {
      clearInterval(poll);
      document.removeEventListener('visibilitychange', handleVisibilityChange);
    };
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
            <svg class="h-4.5 w-4.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="4" y1="12" x2="20" y2="12"/><line x1="4" y1="6" x2="20" y2="6"/><line x1="4" y1="18" x2="20" y2="18"/>
            </svg>
          </button>
          <span class="ml-2 text-xs font-medium text-zinc-300">Stacks</span>
        </div>
        <div class="flex-1 flex flex-col items-center justify-center text-zinc-500 gap-2.5 p-4 text-center">
          <div class="opacity-25 grayscale">
            <Logo size={32} />
          </div>
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
                class="md:hidden -ml-1 rounded-md p-1.5 text-zinc-400 hover:bg-zinc-800 hover:text-white active:scale-95 transition-all duration-150 shrink-0 cursor-pointer"
              >
                <Menu class="h-4.5 w-4.5" />
              </button>
              <h2 class="text-sm font-semibold text-white tracking-tight truncate">{currentStack.name}</h2>
              <span
                class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[11px] font-medium shrink-0 border transition-colors duration-300 {currentStack.status === 'running' ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' : currentStack.status === 'partial' ? 'bg-amber-500/10 text-amber-400 border-amber-500/20' : currentStack.status === 'stopped' || currentStack.status === 'exited' ? 'bg-red-500/10 text-red-400 border-red-500/20' : 'bg-zinc-800/80 text-zinc-400 border-zinc-700/60'}"
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
              <DropdownMenu.Root>
                <div class="relative inline-flex items-center rounded-md border border-blue-500/50 bg-blue-600 shadow-xs">
                  <button
                    onclick={() => handleSave(false)}
                    disabled={activeAction !== null || !hasUnsavedChanges}
                    title="Save draft"
                    class="relative inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-white hover:bg-blue-500 active:scale-95 disabled:opacity-40 transition-colors cursor-pointer disabled:cursor-not-allowed"
                  >
                    <Save class="h-3.5 w-3.5" />
                    <span>{activeAction === 'save' ? '...' : 'Save'}</span>
                    {#if hasUnsavedChanges}
                      <span class="absolute -top-1 -right-1 h-2 w-2 rounded-full bg-amber-400 shadow-[0_0_4px_rgba(251,191,36,0.8)]"></span>
                    {/if}
                  </button>
                  <DropdownMenu.Trigger
                    disabled={activeAction !== null}
                    title="Options"
                    class="inline-flex items-center border-l border-blue-700/80 px-1 py-1 text-xs text-white/90 hover:text-white hover:bg-blue-500 active:scale-95 disabled:opacity-40 transition-colors outline-none cursor-pointer disabled:cursor-not-allowed"
                  >
                    <ChevronDown class="h-3.5 w-3.5" />
                  </DropdownMenu.Trigger>
                </div>

                <DropdownMenu.Portal>
                  <DropdownMenu.Content
                    sideOffset={6}
                    align="end"
                    class="z-50 w-max max-w-[calc(100vw-1.5rem)] rounded-md border border-zinc-800 bg-zinc-900 p-1 shadow-xl text-xs divide-y divide-zinc-800 outline-none"
                  >
                    <div class="p-0.5">
                      <DropdownMenu.Item
                        onSelect={() => handleSave(false)}
                        disabled={!hasUnsavedChanges}
                        class="w-full flex items-center gap-2.5 px-2.5 py-2 rounded text-left outline-none transition-colors whitespace-nowrap {!hasUnsavedChanges ? 'opacity-40 cursor-not-allowed text-zinc-500 select-none' : 'text-zinc-300 hover:text-white hover:bg-zinc-800 data-[highlighted]:bg-zinc-800 data-[highlighted]:text-white cursor-pointer'}"
                      >
                        <Save class="h-3.5 w-3.5 text-zinc-400 shrink-0" />
                        <div>
                          <div class="font-medium text-zinc-200">Save Draft</div>
                          <div class="text-[10px] text-zinc-500 font-normal">Save to disk only</div>
                        </div>
                      </DropdownMenu.Item>
                    </div>
                    <div class="p-0.5">
                      <DropdownMenu.Item
                        onSelect={() => handleSave(true)}
                        class="w-full flex items-center gap-2.5 px-2.5 py-2 rounded text-left text-zinc-300 hover:text-white hover:bg-zinc-800 data-[highlighted]:bg-zinc-800 data-[highlighted]:text-white transition-colors cursor-pointer outline-none whitespace-nowrap"
                      >
                        <Rocket class="h-3.5 w-3.5 text-zinc-400 shrink-0" />
                        <div>
                          <div class="font-medium text-zinc-200">Save & Deploy</div>
                          <div class="text-[10px] text-zinc-500 font-normal">Apply via compose up</div>
                        </div>
                      </DropdownMenu.Item>
                    </div>
                  </DropdownMenu.Content>
                </DropdownMenu.Portal>
              </DropdownMenu.Root>

              <button
                onclick={() => (deleteModalOpen = true)}
                disabled={activeAction !== null}
                title="Delete Stack"
                class="rounded-md border border-zinc-800 bg-zinc-900/80 p-1.5 text-zinc-400 hover:text-red-400 hover:bg-red-500/10 hover:border-red-500/30 active:scale-95 disabled:opacity-50 transition-all duration-150 cursor-pointer disabled:cursor-not-allowed"
              >
                <Trash2 class="h-3.5 w-3.5" />
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
                class="flex items-center justify-center gap-1.5 rounded-md border border-emerald-500/40 bg-emerald-500/10 px-2.5 py-1.5 sm:py-1 text-xs text-emerald-300 hover:bg-emerald-500/20 active:scale-[0.97] disabled:opacity-50 transition-all duration-150 cursor-pointer disabled:cursor-not-allowed"
              >
                <RefreshCw class="h-3.5 w-3.5 shrink-0 text-emerald-400 {activeAction === 'update' ? 'animate-spin' : ''}" />
                <span>{activeAction === 'update' ? 'Updating' : 'Update'}</span>
              </button>
              <button
                onclick={() => handleAction('restart')}
                disabled={activeAction !== null}
                title="Restart Stack"
                class="flex items-center justify-center gap-1.5 rounded-md border border-zinc-800 bg-zinc-900/80 px-2.5 py-1.5 sm:py-1 text-xs text-zinc-300 hover:text-white hover:bg-zinc-800 hover:border-zinc-700 active:scale-[0.97] disabled:opacity-50 transition-all duration-150 cursor-pointer disabled:cursor-not-allowed"
              >
                <RotateCw class="h-3.5 w-3.5 shrink-0 text-zinc-400 {activeAction === 'restart' ? 'animate-spin motion-reduce:animate-none text-amber-400' : ''}" />
                <span>{activeAction === 'restart' ? 'Restarting' : 'Restart'}</span>
              </button>
              <button
                onclick={() => {
                  if (currentStack?.name?.toLowerCase() === 'composebox') {
                    stopModalOpen = true;
                  } else {
                    handleAction('down');
                  }
                }}
                disabled={activeAction !== null}
                title="Stop Stack"
                class="group flex items-center justify-center gap-1.5 rounded-md border border-zinc-800 bg-zinc-900/80 px-2.5 py-1.5 sm:py-1 text-xs text-zinc-300 hover:text-red-400 hover:bg-red-500/10 hover:border-red-500/30 active:scale-[0.97] disabled:opacity-50 transition-all duration-150 cursor-pointer disabled:cursor-not-allowed"
              >
                <Square class="h-3.5 w-3.5 shrink-0 transition-colors duration-150 {activeAction === 'down' ? 'animate-pulse text-red-400' : 'text-zinc-400 group-hover:text-red-400'}" />
                <span>{activeAction === 'down' ? 'Stopping' : 'Stop'}</span>
              </button>
            {:else}
              <button
                onclick={() => handleAction('up')}
                disabled={activeAction !== null}
                title="Start Stack"
                class="flex items-center justify-center gap-1.5 rounded-md border border-emerald-500/40 bg-emerald-500/10 px-2.5 py-1.5 sm:py-1 text-xs text-emerald-300 hover:bg-emerald-500/20 active:scale-[0.97] disabled:opacity-50 transition-all duration-150 cursor-pointer disabled:cursor-not-allowed"
              >
                <Play class="h-3.5 w-3.5 shrink-0 text-emerald-400 {activeAction === 'up' ? 'animate-pulse' : ''}" />
                <span>{activeAction === 'up' ? 'Starting' : 'Start'}</span>
              </button>
              <button
                onclick={() => handleAction('pull')}
                disabled={activeAction !== null}
                title="Pull Images"
                class="flex items-center justify-center gap-1.5 rounded-md border border-zinc-800 bg-zinc-900/80 px-2.5 py-1.5 sm:py-1 text-xs text-zinc-300 hover:text-white hover:bg-zinc-800 hover:border-zinc-700 active:scale-[0.97] disabled:opacity-50 transition-all duration-150 cursor-pointer disabled:cursor-not-allowed"
              >
                <Download class="h-3.5 w-3.5 shrink-0 text-zinc-400 {activeAction === 'pull' ? 'animate-bounce motion-reduce:animate-none' : ''}" />
                <span>{activeAction === 'pull' ? 'Pulling' : 'Pull'}</span>
              </button>
            {/if}

            <!-- Save & Deploy Split Dropdown -->
            <DropdownMenu.Root>
              <div class="relative hidden sm:inline-flex items-center rounded-md border border-blue-500/50 bg-blue-600 shadow-xs">
                <button
                  onclick={() => handleSave(false)}
                  disabled={activeAction !== null || !hasUnsavedChanges}
                  title="Save draft to disk (Ctrl+S)"
                  class="relative inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-white hover:bg-blue-500 active:scale-[0.98] disabled:opacity-40 disabled:hover:bg-blue-600 transition-colors cursor-pointer disabled:cursor-not-allowed"
                >
                  <Save class="h-3.5 w-3.5" />
                  <span>{activeAction === 'save' ? 'Saving...' : 'Save'}</span>
                  {#if hasUnsavedChanges}
                    <span class="absolute -top-1 -right-1 h-2 w-2 rounded-full bg-amber-400 shadow-[0_0_4px_rgba(251,191,36,0.8)]"></span>
                  {/if}
                </button>

                <DropdownMenu.Trigger
                  disabled={activeAction !== null}
                  title="Save & Deploy options"
                  class="inline-flex items-center border-l border-blue-700/80 px-1.5 py-1 text-xs text-white/90 hover:text-white hover:bg-blue-500 active:scale-[0.98] disabled:opacity-40 transition-colors outline-none cursor-pointer disabled:cursor-not-allowed"
                >
                  <ChevronDown class="h-3.5 w-3.5" />
                </DropdownMenu.Trigger>
              </div>

              <DropdownMenu.Portal>
                <DropdownMenu.Content
                  sideOffset={6}
                  align="end"
                  class="z-50 w-max max-w-[calc(100vw-2rem)] rounded-md border border-zinc-800 bg-zinc-900 p-1 shadow-xl text-xs divide-y divide-zinc-800 outline-none"
                >
                  <div class="p-0.5">
                    <DropdownMenu.Item
                      onSelect={() => handleSave(false)}
                      disabled={!hasUnsavedChanges}
                      class="w-full flex items-center justify-between gap-4 px-2.5 py-2 rounded text-left outline-none transition-colors whitespace-nowrap {!hasUnsavedChanges ? 'opacity-40 cursor-not-allowed text-zinc-500 select-none' : 'text-zinc-300 hover:text-white hover:bg-zinc-800 data-[highlighted]:bg-zinc-800 data-[highlighted]:text-white cursor-pointer'}"
                    >
                      <div class="flex items-center gap-2.5">
                        <Save class="h-3.5 w-3.5 text-zinc-400 shrink-0" />
                        <div>
                          <div class="font-medium text-zinc-200">Save Draft</div>
                          <div class="text-[10px] text-zinc-500 font-normal">Save to disk only</div>
                        </div>
                      </div>
                      <kbd class="px-1.5 py-0.5 text-[10px] font-mono text-zinc-400 bg-zinc-800 rounded border border-zinc-700/60 shadow-2xs shrink-0 ml-3">Ctrl+S</kbd>
                    </DropdownMenu.Item>
                  </div>

                  <div class="p-0.5">
                    <DropdownMenu.Item
                      onSelect={() => handleSave(true)}
                      class="w-full flex items-center justify-between gap-4 px-2.5 py-2 rounded text-left text-zinc-300 hover:text-white hover:bg-zinc-800 data-[highlighted]:bg-zinc-800 data-[highlighted]:text-white cursor-pointer outline-none transition-colors whitespace-nowrap"
                    >
                      <div class="flex items-center gap-2.5">
                        <Rocket class="h-3.5 w-3.5 text-zinc-400 shrink-0" />
                        <div>
                          <div class="font-medium text-zinc-200">Save & Deploy</div>
                          <div class="text-[10px] text-zinc-500 font-normal">Apply via compose up</div>
                        </div>
                      </div>
                      <kbd class="px-1.5 py-0.5 text-[10px] font-mono text-zinc-400 bg-zinc-800 rounded border border-zinc-700/60 shadow-2xs shrink-0 ml-3">Ctrl+Shift+S</kbd>
                    </DropdownMenu.Item>
                  </div>
                </DropdownMenu.Content>
              </DropdownMenu.Portal>
            </DropdownMenu.Root>

            <button
              onclick={() => (deleteModalOpen = true)}
              disabled={activeAction !== null}
              title="Delete Stack"
              class="hidden sm:inline-flex rounded-md border border-zinc-800 bg-zinc-900/80 p-1.5 text-zinc-400 hover:text-red-400 hover:bg-red-500/10 hover:border-red-500/30 active:scale-95 disabled:opacity-50 transition-all duration-150 cursor-pointer disabled:cursor-not-allowed shadow-[inset_0_1px_0_0_rgba(255,255,255,0.04)]"
            >
              <Trash2 class="h-3.5 w-3.5" />
            </button>
          </div>
        </div>

        <!-- Navigation tabs -->
        <nav class="flex items-center justify-between border-b border-zinc-800/80 bg-zinc-900/20 px-3.5 sm:px-5 shrink-0 overflow-x-auto">
          <div class="flex gap-5 sm:gap-6">
            <button
              onclick={() => (tab = 'containers')}
              class="border-b-2 py-2.5 text-xs whitespace-nowrap transition-colors duration-150 cursor-pointer {tab === 'containers' ? 'border-blue-500 text-white font-medium' : 'border-transparent text-zinc-400 hover:text-zinc-200'}"
            >
              Containers
            </button>
            <button
              onclick={() => (tab = 'compose')}
              class="border-b-2 py-2.5 text-xs whitespace-nowrap transition-colors duration-150 cursor-pointer {tab === 'compose' ? 'border-blue-500 text-white font-medium' : 'border-transparent text-zinc-400 hover:text-zinc-200'}"
            >
              compose.yaml
            </button>
            <button
              onclick={() => (tab = 'env')}
              class="border-b-2 py-2.5 text-xs whitespace-nowrap transition-colors duration-150 cursor-pointer {tab === 'env' ? 'border-blue-500 text-white font-medium' : 'border-transparent text-zinc-400 hover:text-zinc-200'}"
            >
              .env
            </button>
            <button
              onclick={() => (tab = 'logs')}
              class="border-b-2 py-2.5 text-xs whitespace-nowrap transition-colors duration-150 cursor-pointer {tab === 'logs' ? 'border-blue-500 text-white font-medium' : 'border-transparent text-zinc-400 hover:text-zinc-200'}"
            >
              Logs
            </button>
          </div>

          <button
            onclick={() => (consoleOpen = !consoleOpen)}
            class="flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs transition-all duration-150 cursor-pointer {consoleOpen ? 'bg-zinc-800 text-white border border-zinc-700/80 shadow-xs' : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/50 border border-transparent'}"
            title="Toggle Action Console Drawer"
          >
            <TerminalIcon class="h-3.5 w-3.5" />
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
          onClear={handleClearConsole}
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
    {#if currentStack?.name?.toLowerCase() === 'composebox'}
      <div class="rounded-md bg-red-500/10 border border-red-500/20 p-2.5 text-[11px] text-red-300 mb-4 leading-relaxed">
        ⚠️ <strong>Extreme Warning:</strong> This is the Composebox application stack. Deleting it will permanently erase its files and take down this management interface.
      </div>
    {/if}
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

  <!-- Stop Warning Modal for Self-Hosted Stack -->
  <Modal open={stopModalOpen} onClose={() => (stopModalOpen = false)} maxWidth="max-w-md">
    <div class="space-y-4">
      <!-- Header -->
      <div class="flex items-start gap-3.5">
        <div class="relative rounded-xl border border-amber-500/30 bg-amber-500/10 p-2.5 text-zinc-100 shrink-0 shadow-inner">
          <Logo size={24} />
          <span class="absolute -bottom-1 -right-1 flex h-4 w-4 items-center justify-center rounded-full bg-amber-500 text-zinc-950 font-black text-[10px] shadow-sm">
            !
          </span>
        </div>
        <div class="flex-1 min-w-0">
          <h3 class="text-sm font-semibold text-white">Stop Composebox Manager?</h3>
          <p class="text-xs text-zinc-400 mt-1 leading-relaxed">
            This stack runs the active Composebox dashboard. Stopping it will <strong class="text-zinc-200">terminate this web interface immediately</strong>.
          </p>
        </div>
      </div>

      <!-- Recovery Notice Box -->
      <div class="rounded-lg border border-zinc-800 bg-zinc-950/80 p-3 space-y-2">
        <div class="flex items-center justify-between text-[11px] text-zinc-400">
          <span class="font-medium flex items-center gap-1.5">
            <svg class="h-3 w-3 text-zinc-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>
            Host Recovery Command
          </span>
          <button
            type="button"
            onclick={copyRecoveryCommand}
            class="text-[10px] text-zinc-400 hover:text-white px-2 py-0.5 rounded hover:bg-zinc-800 border border-zinc-800 hover:border-zinc-700 transition-colors"
          >
            {copiedRecovery ? 'Copied!' : 'Copy'}
          </button>
        </div>
        <div class="font-mono text-[11px] text-amber-200/90 bg-zinc-900/90 rounded px-2.5 py-1.5 border border-zinc-800 select-all break-all">
          cd {currentStack?.path || '/home/ubuntu/services/composebox'} && docker compose up -d
        </div>
        <p class="text-[11px] text-zinc-500 leading-normal">
          You will need SSH / server terminal access to run this command and bring Composebox back online.
        </p>
      </div>

      <!-- Footer Action Buttons -->
      <div class="flex items-center justify-end gap-2 pt-2 border-t border-zinc-800/80">
        <button
          type="button"
          onclick={() => (stopModalOpen = false)}
          class="rounded-lg border border-zinc-700 bg-zinc-800/80 px-3.5 py-1.5 text-xs font-medium text-zinc-200 hover:bg-zinc-700 hover:text-white active:scale-95 transition-all duration-150"
        >
          Cancel (Keep Running)
        </button>
        <button
          type="button"
          onclick={() => {
            stopModalOpen = false;
            handleAction('down');
          }}
          class="flex items-center gap-1.5 rounded-lg border border-red-500/40 bg-red-600/90 px-3.5 py-1.5 text-xs font-medium text-white hover:bg-red-600 active:scale-95 shadow-sm transition-all duration-150"
        >
          <svg class="h-3.5 w-3.5 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="14" height="14" x="5" y="5" rx="2"/></svg>
          <span>Stop Anyway</span>
        </button>
      </div>
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
