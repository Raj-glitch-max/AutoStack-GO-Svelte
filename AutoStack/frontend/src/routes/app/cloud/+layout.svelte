<script lang="ts">
  import { page } from "$app/stores";

  const navItems = [
    { href: "/app/cloud", label: "Overview", exact: true },
    { href: "/app/cloud/accounts", label: "Cloud Accounts" },
    { href: "/app/cloud/targets", label: "Deployment Targets" },
    { href: "/app/cloud/deploy", label: "First Deployment" },
  ];

  function isActive(href: string, exact: boolean | undefined): boolean {
    if (exact) return $page.url.pathname === href;
    return $page.url.pathname.startsWith(href);
  }
</script>

<!-- This layout is rendered inside the parent absolutely-positioned container
     (see /app/+layout.svelte).  Using min-h-screen here would push the
     content past the parent's height and collapse the side nav.  Use h-full
     to fill the available space instead. -->
<div class="flex h-full bg-gray-50 dark:bg-gray-900">
  <aside class="w-52 flex-shrink-0 border-r border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 pt-6 overflow-y-auto">
    <nav class="space-y-1 px-3">
      <p class="px-2 pb-2 text-xs font-semibold uppercase tracking-widest text-gray-400 dark:text-gray-500">
        Cloud
      </p>
      {#each navItems as item}
        <a
          href={item.href}
          class="flex items-center rounded-md px-3 py-2 text-sm font-medium transition-colors
            {isActive(item.href, item.exact)
              ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
              : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700'}"
        >
          {item.label}
        </a>
      {/each}
    </nav>
  </aside>

  <main class="flex-1 overflow-y-auto">
    <slot />
  </main>
</div>
