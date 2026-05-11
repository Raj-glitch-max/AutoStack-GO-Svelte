<script lang="ts">
  import { Button, Textarea, Card, Badge, Spinner } from "flowbite-svelte";
  import { Sparkles, ArrowRight, Check, AlertTriangle, Info, ThumbsUp, ThumbsDown } from "lucide-svelte";
  import { fade, slide } from "svelte/transition";
  import { createEventDispatcher } from "svelte";
  import { pb } from "$lib/pocketbase";
  import toast from "svelte-french-toast";

  const dispatch = createEventDispatcher();

  let description = '';
  let loading = false;
  let recommendation: any = null;
  let error = '';
  let feedbackSent = false;

  async function getRecommendation() {
    if (!description.trim()) return;
    
    loading = true;
    error = '';
    recommendation = null;
    feedbackSent = false;
    
    try {
      const response = await fetch(`${pb.baseUrl}/api/intelligence/recommend`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': pb.authStore.token
        },
        body: JSON.stringify({ description })
      });
      
      if (!response.ok) {
        const errData = await response.json().catch(() => ({}));
        throw new Error(errData.error || 'AI failed to generate recommendation');
      }
      
      recommendation = await response.json();
    } catch (err: any) {
      error = err.message;
    } finally {
      loading = false;
    }
  }

  function applyRecommendation() {
    if (recommendation) {
      dispatch('apply', recommendation);
    }
  }

  async function sendFeedback(helpful: boolean) {
    feedbackSent = true;
    try {
      await fetch(`${pb.baseUrl}/api/intelligence/feedback`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': pb.authStore.token
        },
        body: JSON.stringify({ description, recommendation, helpful, type: 'advisor' })
      });
    } catch { /* best-effort */ }
    toast.success(helpful ? 'Thanks for the feedback!' : "Got it — we'll improve this.");
  }
</script>

<div class="space-y-6">
  <div class="space-y-2">
    <div class="flex items-center space-x-2 text-primary-600 dark:text-primary-400">
      <Sparkles size={18} />
      <span class="font-bold uppercase tracking-wider text-xs">AI Deployment Advisor</span>
    </div>
    <p class="text-sm text-gray-500 dark:text-gray-400">Describe your app and we'll suggest the best AWS setup.</p>
    
    <div class="relative">
      <Textarea 
        bind:value={description}
        placeholder="e.g., I'm building a high-traffic React frontend with a Node.js API and a PostgreSQL database. I expect around 10k users per day in Europe."
        rows={4}
        class="bg-white/50 dark:bg-gray-800/50 backdrop-blur-sm border-gray-200 dark:border-gray-700 focus:ring-primary-500 rounded-xl"
      />
      
      <div class="absolute bottom-3 right-3">
        <Button size="xs" color="primary" on:click={getRecommendation} disabled={loading || !description.trim()}>
          {#if loading}
            <Spinner size="3" class="mr-2" />
            Analyzing...
          {:else}
            Get Recommendation
          {/if}
        </Button>
      </div>
    </div>
  </div>

  {#if error}
    <div class="p-4 bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 rounded-xl text-sm border border-red-100 dark:border-red-900/30" transition:fade>
      <AlertTriangle size={16} class="inline mr-2" />
      {error}
    </div>
  {/if}

  {#if recommendation}
    <div transition:slide>
      <Card class="w-full max-w-none border-2 border-primary-500/30 bg-primary-50/10 dark:bg-primary-900/10 shadow-xl overflow-hidden">
        <div class="p-5 space-y-4">
          <div class="flex justify-between items-start">
            <div class="space-y-1">
              <div class="flex items-center space-x-2">
                <Badge color="primary" class="rounded-full px-2">Recommended Setup</Badge>
                <span class="text-xs text-gray-400 dark:text-gray-500">Confidence: High</span>
              </div>
              <h3 class="text-xl font-bold dark:text-white uppercase tracking-tight">
                {recommendation.blueprint_id.replace('-', ' ')}
              </h3>
            </div>
            <div class="text-right">
              <p class="text-xs text-gray-500 dark:text-gray-400 uppercase font-bold tracking-widest">Est. Monthly</p>
              <p class="text-xl font-black text-primary-600 dark:text-primary-400">{recommendation.estimated_cost}</p>
            </div>
          </div>

          <div class="grid grid-cols-2 gap-4">
            <div class="p-3 bg-white dark:bg-gray-800 rounded-xl border border-gray-100 dark:border-gray-700">
              <p class="text-[10px] text-gray-500 uppercase font-bold mb-1">Region</p>
              <p class="text-sm font-medium dark:text-white">{recommendation.region}</p>
            </div>
            <div class="p-3 bg-white dark:bg-gray-800 rounded-xl border border-gray-100 dark:border-gray-700">
              <p class="text-[10px] text-gray-500 uppercase font-bold mb-1">Instance Size</p>
              <p class="text-sm font-medium dark:text-white">{recommendation.instance_size}</p>
            </div>
          </div>

          <div class="space-y-2">
            <p class="text-[10px] text-gray-500 uppercase font-bold flex items-center">
              <Info size={12} class="mr-1" /> Reasoning
            </p>
            <p class="text-sm text-gray-600 dark:text-gray-400 leading-relaxed italic">
              "{recommendation.reasoning}"
            </p>
          </div>

          {#if recommendation.tradeoffs && recommendation.tradeoffs.length > 0}
            <div class="space-y-2">
              <p class="text-[10px] text-gray-500 uppercase font-bold">Key Trade-offs</p>
              <ul class="grid grid-cols-1 gap-2">
                {#each recommendation.tradeoffs as tradeoff}
                  <li class="text-[11px] text-gray-500 dark:text-gray-500 flex items-start">
                    <span class="mr-2 text-primary-500">•</span>
                    {tradeoff}
                  </li>
                {/each}
              </ul>
            </div>
          {/if}

          <Button color="primary" class="w-full group mt-2" on:click={applyRecommendation}>
            Apply Recommended Settings
            <Check size={16} class="ml-2 group-hover:scale-125 transition-transform" />
          </Button>

          <!-- Feedback -->
          {#if !feedbackSent}
            <div class="flex items-center justify-center gap-3 text-xs text-gray-400 pt-1">
              <span>Was this helpful?</span>
              <button on:click={() => sendFeedback(true)} class="hover:text-green-500 transition-colors" aria-label="Yes">
                <ThumbsUp size={14} />
              </button>
              <button on:click={() => sendFeedback(false)} class="hover:text-red-400 transition-colors" aria-label="No">
                <ThumbsDown size={14} />
              </button>
            </div>
          {:else}
            <p class="text-center text-xs text-gray-400">Thanks for the feedback ✓</p>
          {/if}
        </div>
      </Card>
    </div>
  {/if}
</div>
