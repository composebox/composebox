<script>
  import { onDestroy, tick, untrack } from 'svelte';

  let { stackName } = $props();

  const MAX_LOG_CHARS = 200000;
  let logs = $state('');
  let logSource = null;
  let termEl = $state(null);

  function connect(targetStack) {
    if (logSource) {
      logSource.close();
    }
    logs = `[Connected to ${targetStack} logs]\n`;
    logSource = new EventSource(`/api/stacks/${encodeURIComponent(targetStack)}/logs`);

    logSource.onmessage = async (e) => {
      const cleaned = e.data.replace(/\x1b\[[0-9;?]*[ -/]*[@-~]/g, '');
      let next = logs + cleaned + '\n';
      if (next.length > MAX_LOG_CHARS) {
        next = next.slice(next.length - MAX_LOG_CHARS);
      }
      logs = next;
      await tick();
      if (termEl) {
        termEl.scrollTop = termEl.scrollHeight;
      }
    };

    logSource.onerror = () => {
      if (!logs.includes('Disconnected')) {
        logs += '\n[Disconnected: No running containers producing logs. Start stack to stream logs.]\n';
      }
      logSource.close();
    };
  }

  $effect(() => {
    const target = stackName;
    if (target) {
      untrack(() => connect(target));
    }
  });

  onDestroy(() => {
    if (logSource) {
      logSource.close();
    }
  });
</script>

<div class="flex-1 flex flex-col p-3 sm:p-5 overflow-hidden">
  <div class="flex justify-end pb-2">
    <button
      onclick={() => (logs = '')}
      class="rounded border border-zinc-700/80 px-2.5 py-1 sm:py-0.5 text-xs text-zinc-300 hover:bg-zinc-800 transition-colors"
    >
      Clear
    </button>
  </div>
  <div
    bind:this={termEl}
    class="flex-1 rounded-lg border border-zinc-800 bg-zinc-950 p-3.5 sm:p-4 font-mono text-xs leading-relaxed text-zinc-300 overflow-y-auto whitespace-pre-wrap break-all"
  >
    {logs || 'Connecting to logs...'}
  </div>
</div>
