<script>
  export let size = 16;
  export let className = '';
</script>

<svg
  xmlns="http://www.w3.org/2000/svg"
  viewBox="0 0 24 24"
  width={size}
  height={size}
  class={className}
  fill="none"
>
  <defs>
    <linearGradient id="cb-logo-top" x1="3.34" y1="2" x2="20.66" y2="12" gradientUnits="userSpaceOnUse">
      <stop offset="0" stop-color="#38bdf8" />
      <stop offset="1" stop-color="#0284c7" />
    </linearGradient>
    <linearGradient id="cb-logo-left" x1="3.34" y1="7" x2="12" y2="22" gradientUnits="userSpaceOnUse">
      <stop offset="0" stop-color="#0284c7" />
      <stop offset="1" stop-color="#0369a1" />
    </linearGradient>
    <linearGradient id="cb-logo-right" x1="12" y1="7" x2="20.66" y2="22" gradientUnits="userSpaceOnUse">
      <stop offset="0" stop-color="#0ea5e9" />
      <stop offset="1" stop-color="#075985" />
    </linearGradient>
  </defs>

  <!-- 1. Top in back as whole cube silhouette -->
  <path d="M12 2L20.66 7V17L12 22L3.34 17V7Z" fill="url(#cb-logo-top)" stroke="#38bdf8" stroke-width="1.25" stroke-linejoin="round" />

  <!-- 2. Left face on top -->
  <path d="M3.34 7L12 12V22L3.34 17Z" fill="url(#cb-logo-left)" stroke="#0284c7" stroke-width="1.25" stroke-linejoin="round" />

  <!-- 3. Right face on top -->
  <path d="M12 12L20.66 7V17L12 22Z" fill="url(#cb-logo-right)" stroke="#0ea5e9" stroke-width="1.25" stroke-linejoin="round" />
</svg>
