<script lang="ts">
  import { onMount } from 'svelte';
  import { fade, scale } from 'svelte/transition';
  import ArchitectureAnimation from '$lib/components/architecture/ArchitectureAnimation.svelte';
  import { Button } from 'flowbite-svelte';
  import { Play, Pause, RotateCcw } from 'lucide-svelte';

  let showAnimation = false;
  let isPlaying = false;

  function startAnimation() {
    showAnimation = true;
    isPlaying = true;
  }

  function handleAnimationComplete() {
    showAnimation = false;
    isPlaying = false;
  }

  function resetAnimation() {
    showAnimation = false;
    isPlaying = false;
    setTimeout(() => startAnimation(), 100);
  }
</script>

<svelte:head>
  <title>AutoStack Architecture - End-to-End System Animation</title>
  <meta name="description" content="Interactive visualization of AutoStack's complete architecture including Kubernetes, AWS, Terraform, and real-time monitoring systems." />
</svelte:head>

<div class="min-h-screen bg-gradient-to-br from-slate-900 via-purple-900 to-slate-900">
  <!-- Hero Section -->
  <div class="relative overflow-hidden">
    <div class="absolute inset-0 bg-gradient-to-br from-slate-800/20 to-slate-900/20"></div>
    
    <div class="relative max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 pt-20 pb-16">
      <div class="text-center">
        <h1 class="text-4xl md:text-6xl font-bold text-white mb-6">
          AutoStack
          <span class="bg-gradient-to-r from-cyan-400 to-blue-500 bg-clip-text text-transparent">
            Architecture
          </span>
        </h1>
        <p class="text-xl text-gray-300 mb-8 max-w-3xl mx-auto">
          Experience the complete end-to-end deployment pipeline with Kubernetes orchestration, 
          AWS infrastructure provisioning, and real-time monitoring in an interactive animation.
        </p>
        
        <div class="flex justify-center gap-4">
          {#if !showAnimation}
            <Button 
              color="primary" 
              size="lg" 
              on:click={startAnimation}
              class="bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-600 hover:to-blue-700"
            >
              <Play class="w-5 h-5 mr-2" />
              Start Architecture Demo
            </Button>
          {:else}
            <div class="flex items-center gap-4 px-6 py-3 bg-slate-800/50 rounded-lg border border-cyan-400/30">
              <div class="w-3 h-3 bg-cyan-400 rounded-full animate-pulse"></div>
              <span class="text-cyan-400 font-medium">Architecture Animation Running...</span>
            </div>
          {/if}
        </div>
      </div>
    </div>
  </div>

  <!-- Architecture Stats -->
  <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-16">
    <div class="grid grid-cols-2 md:grid-cols-4 gap-8 text-center">
      <div class="bg-white/10 backdrop-blur-sm rounded-lg p-6 border border-white/20">
        <div class="text-3xl font-bold text-cyan-400 mb-2">15+</div>
        <div class="text-gray-300">Microservices</div>
      </div>
      <div class="bg-white/10 backdrop-blur-sm rounded-lg p-6 border border-white/20">
        <div class="text-3xl font-bold text-blue-400 mb-2">3</div>
        <div class="text-gray-300">Cloud Platforms</div>
      </div>
      <div class="bg-white/10 backdrop-blur-sm rounded-lg p-6 border border-white/20">
        <div class="text-3xl font-bold text-purple-400 mb-2">8</div>
        <div class="text-gray-300">Data Flows</div>
      </div>
      <div class="bg-white/10 backdrop-blur-sm rounded-lg p-6 border border-white/20">
        <div class="text-3xl font-bold text-green-400 mb-2">Real-time</div>
        <div class="text-gray-300">Monitoring</div>
      </div>
    </div>
  </div>

  <!-- Technology Stack -->
  <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-16">
    <h2 class="text-3xl font-bold text-white text-center mb-12">
      Complete Technology Stack
    </h2>
    
    <div class="grid grid-cols-1 md:grid-cols-3 gap-8">
      <!-- Frontend Stack -->
      <div class="bg-white/5 backdrop-blur-sm rounded-xl p-6 border border-white/10">
        <h3 class="text-xl font-semibold text-cyan-400 mb-4">Frontend Layer</h3>
        <div class="space-y-3">
          <div class="flex items-center gap-3">
            <div class="w-8 h-8 bg-orange-500 rounded-lg flex items-center justify-center">
              <span class="text-white text-sm font-bold">S</span>
            </div>
            <span class="text-gray-300">SvelteKit Framework</span>
          </div>
          <div class="flex items-center gap-3">
            <div class="w-8 h-8 bg-blue-500 rounded-lg flex items-center justify-center">
              <span class="text-white text-sm font-bold">T</span>
            </div>
            <span class="text-gray-300">Tailwind CSS</span>
          </div>
          <div class="flex items-center gap-3">
            <div class="w-8 h-8 bg-purple-500 rounded-lg flex items-center justify-center">
              <span class="text-white text-sm font-bold">F</span>
            </div>
            <span class="text-gray-300">Framer Motion</span>
          </div>
          <div class="flex items-center gap-3">
            <div class="w-8 h-8 bg-green-500 rounded-lg flex items-center justify-center">
              <span class="text-white text-sm font-bold">W</span>
            </div>
            <span class="text-gray-300">WebSocket Real-time</span>
          </div>
        </div>
      </div>

      <!-- Backend Stack -->
      <div class="bg-white/5 backdrop-blur-sm rounded-xl p-6 border border-white/10">
        <h3 class="text-xl font-semibold text-blue-400 mb-4">Backend Layer</h3>
        <div class="space-y-3">
          <div class="flex items-center gap-3">
            <div class="w-8 h-8 bg-cyan-500 rounded-lg flex items-center justify-center">
              <span class="text-white text-sm font-bold">G</span>
            </div>
            <span class="text-gray-300">Go Runtime</span>
          </div>
          <div class="flex items-center gap-3">
            <div class="w-8 h-8 bg-indigo-500 rounded-lg flex items-center justify-center">
              <span class="text-white text-sm font-bold">P</span>
            </div>
            <span class="text-gray-300">PocketBase Database</span>
          </div>
          <div class="flex items-center gap-3">
            <div class="w-8 h-8 bg-orange-600 rounded-lg flex items-center justify-center">
              <span class="text-white text-sm font-bold">T</span>
            </div>
            <span class="text-gray-300">Terraform Executor</span>
          </div>
          <div class="flex items-center gap-3">
            <div class="w-8 h-8 bg-red-500 rounded-lg flex items-center justify-center">
              <span class="text-white text-sm font-bold">A</span>
            </div>
            <span class="text-gray-300">AWS SDK Integration</span>
          </div>
        </div>
      </div>

      <!-- Infrastructure Stack -->
      <div class="bg-white/5 backdrop-blur-sm rounded-xl p-6 border border-white/10">
        <h3 class="text-xl font-semibold text-green-400 mb-4">Infrastructure Layer</h3>
        <div class="space-y-3">
          <div class="flex items-center gap-3">
            <div class="w-8 h-8 bg-blue-600 rounded-lg flex items-center justify-center">
              <span class="text-white text-sm font-bold">K</span>
            </div>
            <span class="text-gray-300">Kubernetes Orchestration</span>
          </div>
          <div class="flex items-center gap-3">
            <div class="w-8 h-8 bg-yellow-600 rounded-lg flex items-center justify-center">
              <span class="text-white text-sm font-bold">A</span>
            </div>
            <span class="text-gray-300">AWS ECS/Fargate</span>
          </div>
          <div class="flex items-center gap-3">
            <div class="w-8 h-8 bg-gray-600 rounded-lg flex items-center justify-center">
              <span class="text-white text-sm font-bold">D</span>
            </div>
            <span class="text-gray-300">Docker Containers</span>
          </div>
          <div class="flex items-center gap-3">
            <div class="w-8 h-8 bg-purple-600 rounded-lg flex items-center justify-center">
              <span class="text-white text-sm font-bold">S</span>
            </div>
            <span class="text-gray-300">S3 State Backend</span>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- Key Features -->
  <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-16">
    <h2 class="text-3xl font-bold text-white text-center mb-12">
      Architecture Highlights
    </h2>
    
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
      <div class="bg-gradient-to-br from-cyan-500/20 to-blue-600/20 rounded-xl p-6 border border-cyan-500/30">
        <div class="w-12 h-12 bg-cyan-500 rounded-lg flex items-center justify-center mb-4">
          <span class="text-white text-xl">🚀</span>
        </div>
        <h3 class="text-xl font-semibold text-white mb-2">One-Click Deployment</h3>
        <p class="text-gray-300">Deploy to Kubernetes or AWS with a single click. Automated infrastructure provisioning with Terraform.</p>
      </div>

      <div class="bg-gradient-to-br from-purple-500/20 to-pink-600/20 rounded-xl p-6 border border-purple-500/30">
        <div class="w-12 h-12 bg-purple-500 rounded-lg flex items-center justify-center mb-4">
          <span class="text-white text-xl">📊</span>
        </div>
        <h3 class="text-xl font-semibold text-white mb-2">Real-time Monitoring</h3>
        <p class="text-gray-300">Live log streaming, deployment status updates, and infrastructure health monitoring via WebSockets.</p>
      </div>

      <div class="bg-gradient-to-br from-green-500/20 to-emerald-600/20 rounded-xl p-6 border border-green-500/30">
        <div class="w-12 h-12 bg-green-500 rounded-lg flex items-center justify-center mb-4">
          <span class="text-white text-xl">💰</span>
        </div>
        <h3 class="text-xl font-semibold text-white mb-2">Cost Estimation</h3>
        <p class="text-gray-300">Real-time AWS cost estimation using the Pricing API before deployment to optimize resource allocation.</p>
      </div>

      <div class="bg-gradient-to-br from-orange-500/20 to-red-600/20 rounded-xl p-6 border border-orange-500/30">
        <div class="w-12 h-12 bg-orange-500 rounded-lg flex items-center justify-center mb-4">
          <span class="text-white text-xl">🔒</span>
        </div>
        <h3 class="text-xl font-semibold text-white mb-2">Secure Credentials</h3>
        <p class="text-gray-300">AES-256 encrypted credential storage with user isolation and least-privilege IAM policies.</p>
      </div>

      <div class="bg-gradient-to-br from-blue-500/20 to-indigo-600/20 rounded-xl p-6 border border-blue-500/30">
        <div class="w-12 h-12 bg-blue-500 rounded-lg flex items-center justify-center mb-4">
          <span class="text-white text-xl">🔄</span>
        </div>
        <h3 class="text-xl font-semibold text-white mb-2">State Management</h3>
        <p class="text-gray-300">Terraform state versioning with S3 backend and DynamoDB locking for consistent infrastructure management.</p>
      </div>

      <div class="bg-gradient-to-br from-teal-500/20 to-cyan-600/20 rounded-xl p-6 border border-teal-500/30">
        <div class="w-12 h-12 bg-teal-500 rounded-lg flex items-center justify-center mb-4">
          <span class="text-white text-xl">📋</span>
        </div>
        <h3 class="text-xl font-semibold text-white mb-2">Blueprint Templates</h3>
        <p class="text-gray-300">Pre-built infrastructure patterns for ECS web apps, full-stack applications, and static sites.</p>
      </div>
    </div>
  </div>
</div>

{#if showAnimation}
  <ArchitectureAnimation 
    projectName="AutoStack" 
    onComplete={handleAnimationComplete} 
  />
{/if}