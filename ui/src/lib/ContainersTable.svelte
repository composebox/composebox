<script>
  let { containers = [] } = $props();
</script>

<div class="flex-1 overflow-auto p-3 sm:p-5">
  {#if containers.length === 0}
    <div class="rounded-lg border border-zinc-800 bg-zinc-900/40 py-10 text-center text-xs text-zinc-500">
      No containers or services defined in compose file
    </div>
  {:else}
    <!-- Mobile Card List (< sm) -->
    <div class="space-y-2.5 sm:hidden">
      {#each containers as c, i (c.name || i)}
        <div class="rounded-lg border border-zinc-800 bg-zinc-900/50 p-3.5 space-y-2">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <div class="font-medium text-xs text-white truncate">{c.name || '-'}</div>
              {#if c.service && c.service !== c.name}
                <div class="text-[11px] text-zinc-500 truncate">{c.service}</div>
              {/if}
            </div>
            <span class="inline-flex items-center gap-1.5 shrink-0 text-xs">
              <span
                class="h-1.5 w-1.5 rounded-full {c.state?.toLowerCase() === 'running' ? 'bg-emerald-400' : 'bg-red-400'}"
              ></span>
              <span class="text-zinc-200">{c.state || '-'}</span>
              {#if c.health}
                <span class="rounded bg-zinc-800 px-1.5 py-0.5 text-[10px] text-zinc-400">{c.health}</span>
              {/if}
            </span>
          </div>

          <div class="grid grid-cols-1 gap-1 pt-1 border-t border-zinc-800/60 text-[11px]">
            <div class="flex justify-between gap-2">
              <span class="text-zinc-500 shrink-0">Image</span>
              <span class="font-mono text-zinc-300 truncate">{c.image || '-'}</span>
            </div>
            <div class="flex justify-between gap-2">
              <span class="text-zinc-500 shrink-0">Status</span>
              <span class="text-zinc-300 truncate">{c.status || '-'}</span>
            </div>
            {#if c.ports}
              <div class="flex justify-between gap-2">
                <span class="text-zinc-500 shrink-0">Ports</span>
                <span class="font-mono text-zinc-300 truncate">{c.ports}</span>
              </div>
            {/if}
          </div>
        </div>
      {/each}
    </div>

    <!-- Desktop Table (>= sm) -->
    <div class="hidden sm:block overflow-x-auto rounded-lg border border-zinc-800 bg-zinc-900/40">
      <table class="w-full text-left text-xs">
        <thead class="border-b border-zinc-800 bg-zinc-900/80 uppercase text-[10px] text-zinc-400 tracking-wider">
          <tr>
            <th class="py-2.5 px-4 font-semibold">Service / Name</th>
            <th class="py-2.5 px-4 font-semibold">State</th>
            <th class="py-2.5 px-4 font-semibold">Image</th>
            <th class="py-2.5 px-4 font-semibold">Status</th>
            <th class="py-2.5 px-4 font-semibold">Ports</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-zinc-800/60">
          {#each containers as c, i (c.name || i)}
            <tr class="hover:bg-zinc-800/30 transition-colors">
              <td class="py-2.5 px-4 whitespace-nowrap">
                <div class="font-medium text-white">{c.name || '-'}</div>
                {#if c.service && c.service !== c.name}
                  <div class="text-[11px] text-zinc-500">{c.service}</div>
                {/if}
              </td>
              <td class="py-2.5 px-4 whitespace-nowrap">
                <span class="inline-flex items-center gap-1.5">
                  <span
                    class="h-1.5 w-1.5 rounded-full {c.state?.toLowerCase() === 'running' ? 'bg-emerald-400' : 'bg-red-400'}"
                  ></span>
                  <span>{c.state || '-'}</span>
                  {#if c.health}
                    <span class="rounded bg-zinc-800 px-1.5 py-0.5 text-[10px] text-zinc-400">{c.health}</span>
                  {/if}
                </span>
              </td>
              <td class="py-2.5 px-4 font-mono text-[11px] text-zinc-400 whitespace-nowrap">{c.image || '-'}</td>
              <td class="py-2.5 px-4 text-zinc-300 whitespace-nowrap">{c.status || '-'}</td>
              <td class="py-2.5 px-4 font-mono text-[11px] text-zinc-400 whitespace-nowrap">{c.ports || '-'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
