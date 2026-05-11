<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { pb } from '$lib/pocketbase';
  import type { CostAlert } from '$lib/types/cost';
  import { 
    Bell, 
    AlertTriangle, 
    TrendingUp, 
    TrendingDown, 
    Check,
    MoreVertical,
    ExternalLink,
    Clock
  } from 'lucide-svelte';
  import { 
    Dropdown, 
    DropdownItem, 
    Indicator, 
    Badge,
    Button
  } from 'flowbite-svelte';
  import { goto } from '$app/navigation';
  import toast from 'svelte-french-toast';

  let alerts: CostAlert[] = [];
  let unreadCount = 0;
  let loading = false;
  let interval: any;

  async function fetchAlerts() {
    try {
      const response = await fetch(`${pb.baseUrl}/api/alerts`, {
        headers: {
          'Authorization': pb.authStore.token
        }
      });
      if (response.ok) {
        alerts = await response.json();
        updateUnreadCount();
      }
    } catch (err) {
      console.error('Error fetching alerts:', err);
    }
  }

  async function fetchUnreadCount() {
    try {
      const response = await fetch(`${pb.baseUrl}/api/alerts/unread-count`, {
        headers: {
          'Authorization': pb.authStore.token
        }
      });
      if (response.ok) {
        const data = await response.json();
        unreadCount = data.count;
      }
    } catch (err) {
      console.error('Error fetching unread count:', err);
    }
  }

  function updateUnreadCount() {
    unreadCount = alerts.filter(a => !a.acknowledged).length;
  }

  async function acknowledgeAlert(alertId: string) {
    try {
      const response = await fetch(`${pb.baseUrl}/api/alerts/${alertId}/acknowledge`, {
        method: 'POST',
        headers: {
          'Authorization': pb.authStore.token
        }
      });
      if (response.ok) {
        alerts = alerts.map(a => a.id === alertId ? { ...a, acknowledged: true } : a);
        updateUnreadCount();
      }
    } catch (err) {
      console.error('Error acknowledging alert:', err);
      toast.error('Failed to acknowledge alert');
    }
  }

  async function acknowledgeAll() {
    try {
      const response = await fetch(`${pb.baseUrl}/api/alerts/acknowledge-all`, {
        method: 'POST',
        headers: {
          'Authorization': pb.authStore.token
        }
      });
      if (response.ok) {
        alerts = alerts.map(a => ({ ...a, acknowledged: true }));
        updateUnreadCount();
        toast.success('All alerts acknowledged');
      }
    } catch (err) {
      console.error('Error acknowledging all alerts:', err);
      toast.error('Failed to acknowledge all alerts');
    }
  }

  function formatTime(dateStr: string | undefined) {
    if (!dateStr) return 'Unknown';
    const date = new Date(dateStr);
    const now = new Date();
    const diff = now.getTime() - date.getTime();
    
    if (diff < 60000) return 'Just now';
    if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
    if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
    return date.toLocaleDateString();
  }

  onMount(() => {
    fetchAlerts();
    fetchUnreadCount();
    // Poll for new alerts every 5 minutes
    interval = setInterval(() => {
      fetchAlerts();
      fetchUnreadCount();
    }, 5 * 60 * 1000);
  });

  onDestroy(() => {
    if (interval) clearInterval(interval);
  });
</script>

<div class="relative mr-4">
  <button
    id="bell-menu"
    class="relative p-2 text-white hover:bg-primary-700 rounded-full transition-colors active:scale-95"
  >
    <Bell class="w-6 h-6" />
    {#if unreadCount > 0}
      <Indicator
        color="red"
        size="md"
        class="absolute top-1 right-1 border-2 border-primary-600"
      >
        <span class="text-[10px] font-bold">{unreadCount > 9 ? '9+' : unreadCount}</span>
      </Indicator>
    {/if}
  </button>

  <Dropdown placement="bottom" triggeredBy="#bell-menu" class="w-80 max-h-[500px] overflow-y-auto">
    <div class="p-3 border-b dark:border-gray-700 flex justify-between items-center bg-gray-50 dark:bg-gray-800">
      <span class="font-bold text-gray-900 dark:text-white">Notifications</span>
      {#if unreadCount > 0}
        <button 
          on:click={acknowledgeAll}
          class="text-xs text-primary-600 hover:text-primary-700 dark:text-primary-400 font-medium"
        >
          Mark all as read
        </button>
      {/if}
    </div>

    {#if alerts.length === 0}
      <div class="p-8 text-center text-gray-500">
        <Bell class="w-12 h-12 mx-auto mb-2 opacity-20" />
        <p>No notifications yet</p>
      </div>
    {:else}
      <div class="divide-y dark:divide-gray-700">
        {#each alerts as alert}
          <div class="p-4 hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors {alert.acknowledged ? 'opacity-60' : ''}">
            <div class="flex gap-3">
              <div class="mt-1">
                {#if alert.type === 'SPIKE' || alert.type === 'anomaly_detected'}
                  <div class="p-2 bg-red-100 dark:bg-red-900/30 text-red-600 rounded-lg">
                    <TrendingUp class="w-4 h-4" />
                  </div>
                {:else if alert.type === 'DROP'}
                  <div class="p-2 bg-blue-100 dark:bg-blue-900/30 text-blue-600 rounded-lg">
                    <TrendingDown class="w-4 h-4" />
                  </div>
                {:else if alert.type === 'budget_exceeded'}
                  <div class="p-2 bg-amber-100 dark:bg-amber-900/30 text-amber-600 rounded-lg">
                    <AlertTriangle class="w-4 h-4" />
                  </div>
                {:else}
                  <div class="p-2 bg-blue-100 dark:bg-blue-900/30 text-blue-600 rounded-lg">
                    <Bell class="w-4 h-4" />
                  </div>
                {/if}
              </div>
              
              <div class="flex-1 min-w-0">
                <div class="flex justify-between items-start mb-1">
                  <span class="text-sm font-semibold text-gray-900 dark:text-white truncate">
                    {#if alert.type === 'SPIKE'}Cost Spike{:else if alert.type === 'DROP'}Cost Drop{:else if alert.type === 'budget_exceeded'}Budget Alert{:else}Cost Anomaly{/if}
                  </span>
                  <span class="text-[10px] text-gray-500 flex items-center gap-1">
                    <Clock class="w-3 h-3" />
                    {formatTime(alert.created)}
                  </span>
                </div>
                <p class="text-xs text-gray-600 dark:text-gray-400 line-clamp-2 mb-2">
                  {alert.message}
                </p>
                
                <div class="flex items-center justify-between mt-2">
                  <div class="flex gap-2">
                    <Button 
                      size="xs" 
                      color="alternative" 
                      class="px-2 py-1"
                      on:click={() => goto(`/app/projects/${alert.expand?.deployment?.project}/deployments/${alert.deployment}`)}
                    >
                      <ExternalLink class="w-3 h-3 mr-1" /> View
                    </Button>
                    {#if !alert.acknowledged}
                      <Button 
                        size="xs" 
                        color="light" 
                        class="px-2 py-1"
                        on:click={() => acknowledgeAlert(alert.id)}
                      >
                        <Check class="w-3 h-3 mr-1" /> Clear
                      </Button>
                    {/if}
                  </div>
                  {#if alert.variance && alert.variance > 0}
                    <Badge color={alert.variance > 50 ? 'red' : 'yellow'} class="text-[10px]">
                      +{alert.variance.toFixed(0)}%
                    </Badge>
                  {/if}
                </div>
              </div>
            </div>
          </div>
        {/each}
      </div>
    {/if}
    
    <div class="p-2 text-center border-t dark:border-gray-700 bg-gray-50 dark:bg-gray-800">
      <Button 
        variant="link" 
        size="sm" 
        class="text-xs text-gray-500 hover:text-gray-700"
        on:click={() => goto('/app/alerts')}
      >
        View all notifications
      </Button>
    </div>
  </Dropdown>
</div>
