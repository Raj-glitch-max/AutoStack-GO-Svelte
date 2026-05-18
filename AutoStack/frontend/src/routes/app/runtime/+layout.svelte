<script lang="ts">
  import { page } from "$app/stores";

  const navItems = [
    { href: "/app/runtime/deployments", label: "Deployments" },
    { href: "/app/runtime/topology",     label: "Topology" },
    { href: "/app/runtime/truth",        label: "Truth Status" },
    { href: "/app/runtime/observations", label: "Observations" },
    { href: "/app/runtime/drifts",       label: "Drifts" },
    { href: "/app/runtime/incidents",    label: "Incidents" },
    { href: "/app/runtime/export",       label: "Export" },
  ];

  function isActive(href: string): boolean {
    return $page.url.pathname.startsWith(href);
  }
</script>

<div class="flex min-h-screen bg-gray-50 dark:bg-gray-900">
  <aside class="w-52 flex-shrink-0 border-r border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 pt-6">
    <nav class="space-y-1 px-3">
      <p class="px-2 pb-2 text-xs font-semibold uppercase tracking-widest text-gray-400 dark:text-gray-500">
        Runtime
      </p>
      {#each navItems as item}
        <a
          href={item.href}
          class="flex items-center rounded-md px-3 py-2 text-sm font-medium transition-colors
            {isActive(item.href)
              ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
              : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700'}"
        >
          {item.label}
        </a>
      {/each}
    </nav>
  </aside>

  <main class="flex-1 overflow-auto">
    <slot />
  </main>
</div>
