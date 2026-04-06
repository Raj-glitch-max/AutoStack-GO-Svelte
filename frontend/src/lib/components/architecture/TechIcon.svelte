<script lang="ts">
  export let name: string;
  export let size: string = "24";
  
  const sizeNum = parseInt(size);
</script>

<div 
  class="flex items-center justify-center"
  style="width: {size}px; height: {size}px;"
>
  {#if name === 'user'}
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <circle cx="12" cy="8" r="4" fill="#60A5FA"/>
      <path d="M6 21v-2a4 4 0 0 1 4-4h4a4 4 0 0 1 4 4v2" stroke="#60A5FA" stroke-width="2" fill="none"/>
    </svg>
  {:else if name === 'svelte'}
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M20.04 8.92c-1.15-4.65-6.07-7.45-10.99-6.25L4.27 4.49C1.46 5.3-.32 8.28.49 11.09c.32 1.12.98 2.12 1.9 2.87l-.18.36c-1.15 4.65 1.65 9.57 6.3 10.72 1.85.46 3.78.18 5.43-.78l4.78-2.82c2.81-.81 4.59-3.79 3.78-6.6-.32-1.12-.98-2.12-1.9-2.87l.18-.36c1.15-4.65-1.65-9.57-6.3-10.72z" fill="#FF3E00"/>
      <path d="M15.25 16.25c-1.38 1.38-3.62 1.38-5 0s-1.38-3.62 0-5 3.62-1.38 5 0 1.38 3.62 0 5z" fill="#FFFFFF"/>
    </svg>
  {:else if name === 'css'}
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M5 3l1.5 17L12 22l5.5-2L19 3H5z" fill="#1572B6"/>
      <path d="M12 4.5v15l4.5-1.5L17.25 4.5H12z" fill="#33A9DC"/>
      <path d="M12 8.5h4.5l-.25 2H12v2h3.75l-.75 8L12 21.5v-2l2.25-.75.5-5.25H12V8.5z" fill="#FFFFFF"/>
    </svg>
  {:else if name === 'nginx'}
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M12 2L22 7v10l-10 5L2 17V7l10-5z" fill="#009639"/>
      <path d="M8 8v8l4-2V6L8 8z" fill="#FFFFFF"/>
      <path d="M16 8L12 10v8l4-2V8z" fill="#FFFFFF"/>
    </svg>
  {:else if name === 'aws'}
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M6.5 14.5c0 .8.7 1.5 1.5 1.5s1.5-.7 1.5-1.5-.7-1.5-1.5-1.5-1.5.7-1.5 1.5z" fill="#FF9900"/>
      <path d="M14.5 14.5c0 .8.7 1.5 1.5 1.5s1.5-.7 1.5-1.5-.7-1.5-1.5-1.5-1.5.7-1.5 1.5z" fill="#FF9900"/>
      <path d="M4 18c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2v-8c0-1.1-.9-2-2-2H6c-1.1 0-2 .9-2 2v8z" fill="#232F3E"/>
      <path d="M6 6h12v2H6V6z" fill="#FF9900"/>
    </svg>
  {:else if name === 'go'}
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <circle cx="12" cy="12" r="10" fill="#00ADD8"/>
      <path d="M7 10h10v4H7v-4z" fill="#FFFFFF"/>
      <circle cx="9.5" cy="12" r="1" fill="#00ADD8"/>
      <circle cx="14.5" cy="12" r="1" fill="#00ADD8"/>
    </svg>
  {:else if name === 'database'}
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <ellipse cx="12" cy="6" rx="8" ry="3" fill="#336791"/>
      <path d="M4 6v6c0 1.66 3.58 3 8 3s8-1.34 8-3V6" stroke="#336791" stroke-width="2" fill="none"/>
      <path d="M4 12v6c0 1.66 3.58 3 8 3s8-1.34 8-3v-6" stroke="#336791" stroke-width="2" fill="none"/>
    </svg>
  {:else if name === 'terraform'}
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M8 2l6 4v4l-6-4V2z" fill="#623CE4"/>
      <path d="M14 6l6 4v4l-6-4V6z" fill="#623CE4"/>
      <path d="M8 10l6 4v4l-6-4v-4z" fill="#623CE4"/>
      <path d="M2 6l6 4v4l-6-4V6z" fill="#623CE4"/>
    </svg>
  {:else if name === 'kubernetes'}
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <circle cx="12" cy="12" r="10" fill="#326CE5"/>
      <path d="M12 4l2.5 7.5h7.5l-6 4.5 2.5 7.5L12 19l-6 4.5 2.5-7.5-6-4.5h7.5L12 4z" fill="#FFFFFF"/>
    </svg>
  {:else if name === 'server'}
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <rect x="2" y="4" width="20" height="4" rx="1" fill="#6B7280"/>
      <rect x="2" y="10" width="20" height="4" rx="1" fill="#6B7280"/>
      <rect x="2" y="16" width="20" height="4" rx="1" fill="#6B7280"/>
      <circle cx="5" cy="6" r="1" fill="#10B981"/>
      <circle cx="5" cy="12" r="1" fill="#10B981"/>
      <circle cx="5" cy="18" r="1" fill="#10B981"/>
    </svg>
  {:else if name === 'storage'}
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <rect x="3" y="4" width="18" height="16" rx="2" fill="#FF9900"/>
      <path d="M7 8h10M7 12h8M7 16h6" stroke="#FFFFFF" stroke-width="2" stroke-linecap="round"/>
    </svg>
  {:else if name === 'monitor'}
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <rect x="2" y="4" width="20" height="12" rx="2" fill="#1F2937" stroke="#60A5FA" stroke-width="1"/>
      <path d="M6 10l2-2 2 4 2-3 2 2 2-3 2 2" stroke="#10B981" stroke-width="2" fill="none"/>
      <rect x="10" y="16" width="4" height="2" fill="#6B7280"/>
      <rect x="8" y="18" width="8" height="2" fill="#6B7280"/>
    </svg>
  {:else if name === 'dollar'}
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <circle cx="12" cy="12" r="10" fill="#059669"/>
      <path d="M12 6v12M8 9c0-1.1.9-2 2-2h4c1.1 0 2 .9 2 2s-.9 2-2 2h-4c-1.1 0-2 .9-2 2s.9 2 2 2h4c1.1 0 2-.9 2-2" stroke="#FFFFFF" stroke-width="2" stroke-linecap="round"/>
    </svg>
  {:else if name === 'security'}
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M12 2L4 6v5c0 5.55 3.84 10.74 8 12 4.16-1.26 8-6.45 8-12V6l-8-4z" fill="#DC2626"/>
      <path d="M9 12l2 2 4-4" stroke="#FFFFFF" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
    </svg>
  {:else}
    <!-- Default gear icon -->
    <svg width={sizeNum} height={sizeNum} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <circle cx="12" cy="12" r="3" fill="#6B7280"/>
      <path d="M19.4 15c-.3 0-.6-.1-.8-.3l-1.2-1.2c-.4-.4-.4-1 0-1.4s1-.4 1.4 0l1.2 1.2c.4.4.4 1 0 1.4-.2.2-.5.3-.8.3z" fill="#6B7280"/>
    </svg>
  {/if}
</div>