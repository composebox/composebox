<script>
  let { value = $bindable(''), placeholder = '' } = $props();

  function handleKeydown(e) {
    if (e.key === 'Tab') {
      e.preventDefault();
      const target = e.target;
      const start = target.selectionStart;
      const end = target.selectionEnd;
      target.value = target.value.substring(0, start) + '  ' + target.value.substring(end);
      target.selectionStart = target.selectionEnd = start + 2;
      value = target.value;
    }
  }
</script>

<div class="flex-1 flex flex-col p-3 sm:p-5 overflow-hidden">
  <textarea
    bind:value={value}
    onkeydown={handleKeydown}
    {placeholder}
    spellcheck="false"
    autocapitalize="off"
    autocomplete="off"
    autocorrect="off"
    class="flex-1 w-full rounded-lg border border-zinc-800 bg-zinc-900/60 p-3.5 sm:p-4 font-mono text-[13px] sm:text-xs text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none resize-none leading-relaxed"
  ></textarea>
</div>
