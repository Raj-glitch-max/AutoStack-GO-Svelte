<script lang="ts">
  import { onMount } from 'svelte';
  import { fade, scale, fly } from 'svelte/transition';
  import { PROJECT_ARCHITECTURES, type ArchitectureConfig } from '$lib/architecture/projectArchitectures';
  import TechIcon from './TechIcon.svelte';

  export let projectName: string;
  export let onComplete: () => void;

  let currentTime = 0;
  let terminalLineIndex = 0;
  let isComplete = false;
  let particles: Array<{ id: string; x: number; y: number; targetX: number; targetY: number; progress: number }> = [];

  const config: ArchitectureConfig = PROJECT_ARCHITECTURES[projectName];

  onMount(() => {
    if (!config) {
      setTimeout(() => onComplete(), 500);
      return;
    }

    const startTime = Date.now();
    const timer = setInterval(() => {
      const elapsed = (Date.now() - startTime) / 1000;
      currentTime = elapsed;

      if (elapsed >= config.duration) {
        isComplete = true;
        clearInterval(timer);
        // Don't auto-close, let user control when to close
      }
    }, 50);

    // Terminal animation
    const terminalTimer = setInterval(() => {
      if (terminalLineIndex < config.terminalCommands.length - 1) {
        terminalLineIndex++;
      }
    }, 800);

    // Particle animation
    const particleTimer = setInterval(() => {
      updateParticles();
    }, 100);

    return () => {
      clearInterval(timer);
      clearInterval(terminalTimer);
      clearInterval(particleTimer);
    };
  });

  function updateParticles() {
    // Add new particles for active connections (less frequent)
    config.connections.forEach((conn, i) => {
      if (currentTime >= conn.delay + 1.5) {
        const fromNode = config.nodes.find(n => n.id === conn.from);
        const toNode = config.nodes.find(n => n.id === conn.to);
        
        if (fromNode && toNode && Math.random() < 0.15) { // Reduced frequency
          particles = [...particles, {
            id: `particle-${i}-${Date.now()}`,
            x: fromNode.x,
            y: fromNode.y,
            targetX: toNode.x,
            targetY: toNode.y,
            progress: 0
          }];
        }
      }
    });

    // Update existing particles (slower movement)
    particles = particles.map(particle => ({
      ...particle,
      progress: Math.min(particle.progress + 0.02, 1) // Slower movement
    })).filter(particle => particle.progress < 1);
  }

  function getParticlePosition(particle: any) {
    const x = particle.x + (particle.targetX - particle.x) * particle.progress;
    const y = particle.y + (particle.targetY - particle.y) * particle.progress;
    return { x, y };
  }
</script>

{#if config}
<div 
  class="fixed inset-0 z-[300] bg-black"
  in:fade={{ duration: 300 }}
  out:fade={{ duration: 300 }}
>
  <div class="relative w-full h-full overflow-auto">
    <div
      class="bg-black w-full h-full p-4"
      in:scale={{ duration: 500, start: 0.9 }}
    >
      <!-- Header -->
      <div class="absolute top-4 left-0 right-0 text-center z-10">
        <button
          class="absolute top-0 right-4 w-10 h-10 bg-gray-800 hover:bg-gray-700 rounded-full flex items-center justify-center text-gray-400 hover:text-white transition-colors"
          on:click={onComplete}
          title="Close Animation"
        >
          ✕
        </button>
        <h2 class="text-4xl font-bold text-white mb-2">
          AutoStack Architecture Deployment
        </h2>
        <p class="text-gray-400">
          End-to-end infrastructure with Kubernetes & AWS integration
        </p>
      </div>

      <!-- Main Architecture Diagram - Full Screen -->
      <div class="relative w-full h-full bg-black">
        <svg viewBox="0 0 1920 1080" class="w-full h-full" preserveAspectRatio="xMidYMid meet">
            <!-- Background Grid -->
            <defs>
              <pattern id="grid" width="80" height="80" patternUnits="userSpaceOnUse">
                <path d="M 80 0 L 0 0 0 80" fill="none" stroke="rgba(0,255,255,0.05)" stroke-width="1"/>
              </pattern>
            </defs>
            <rect width="100%" height="100%" fill="url(#grid)" />

            <!-- Connections -->
            {#each config.connections as conn, i}
              {@const fromNode = config.nodes.find(n => n.id === conn.from)}
              {@const toNode = config.nodes.find(n => n.id === conn.to)}
              {#if fromNode && toNode}
                {@const isVisible = currentTime >= conn.delay}
                {@const progress = Math.min(1, Math.max(0, (currentTime - conn.delay) / 1.0))}
                
                {#if isVisible}
                  <line
                    x1={fromNode.x}
                    y1={fromNode.y}
                    x2={fromNode.x + (toNode.x - fromNode.x) * progress}
                    y2={fromNode.y + (toNode.y - fromNode.y) * progress}
                    stroke="rgba(0, 255, 255, 0.8)"
                    stroke-width="1.5"
                    stroke-dasharray="5,5"
                    stroke-linecap="round"
                    in:fly={{ duration: 1000, x: -20 }}
                  />
                {/if}
              {/if}
            {/each}

            <!-- Data Flow Indicators -->
            {#each particles as particle (particle.id)}
              {@const pos = getParticlePosition(particle)}
              <circle
                cx={pos.x}
                cy={pos.y}
                r="2"
                fill="rgba(0, 255, 255, 0.9)"
                in:scale={{ duration: 300 }}
                out:scale={{ duration: 300 }}
              >
                <animate attributeName="opacity" values="0.9;0.3;0.9" dur="1.5s" repeatCount="indefinite"/>
              </circle>
            {/each}

            <!-- Nodes -->
            {#each config.nodes as node}
              {@const isVisible = currentTime >= node.delay}
              
              {#if isVisible}
                <g in:scale={{ duration: 800, delay: node.delay * 1000, start: 0 }}>
                  <!-- Node Background -->
                  <rect
                    x={node.x - 80}
                    y={node.y - 50}
                    width="160"
                    height="100"
                    rx="16"
                    fill="rgba(15, 15, 15, 0.95)"
                    stroke="rgba(0, 255, 255, 0.6)"
                    stroke-width="3"
                    filter="drop-shadow(0 6px 20px rgba(0,255,255,0.3))"
                  />
                  
                  <!-- Node Icon -->
                  <foreignObject
                    x={node.x - 24}
                    y={node.y - 30}
                    width="48"
                    height="48"
                  >
                    <div class="flex items-center justify-center w-full h-full">
                      <TechIcon name={node.icon} size="40" />
                    </div>
                  </foreignObject>
                  
                  <!-- Node Label -->
                  <text
                    x={node.x}
                    y={node.y + 25}
                    text-anchor="middle"
                    class="fill-white text-lg font-bold"
                    style="font-family: 'Courier New', monospace;"
                  >
                    {node.label.toUpperCase()}
                  </text>
                  
                  <!-- Status Indicator -->
                  <circle
                    cx={node.x + 65}
                    cy={node.y - 35}
                    r="6"
                    fill="#00FF88"
                  >
                    <animate attributeName="opacity" values="1;0.4;1" dur="2s" repeatCount="indefinite"/>
                  </circle>
                </g>
              {/if}
            {/each}
          </svg>
      </div>

      <!-- Terminal Output - Bottom Left Overlay -->
      <div class="absolute bottom-4 left-4 w-96 bg-black/90 border border-gray-700 rounded-lg p-4 backdrop-blur-sm">
        <div class="flex items-center gap-2 mb-3">
          <div class="w-3 h-3 bg-red-500 rounded-full"></div>
          <div class="w-3 h-3 bg-yellow-500 rounded-full"></div>
          <div class="w-3 h-3 bg-green-500 rounded-full"></div>
          <span class="text-gray-400 text-sm ml-2">AutoStack Terminal</span>
        </div>
        <div class="font-mono text-sm text-cyan-400 h-32 overflow-y-auto">
          {#each config.terminalCommands.slice(0, terminalLineIndex + 1) as command, i}
            <div
              class="mb-1"
              in:fly={{ delay: i * 800, x: -10, duration: 300 }}
            >
              <span class="text-cyan-300">$</span> {command}
            </div>
          {/each}
          {#if terminalLineIndex < config.terminalCommands.length - 1}
            <div class="inline-block w-2 h-4 bg-cyan-400 animate-pulse"></div>
          {/if}
        </div>
      </div>

      <!-- Status Panel - Bottom Right Overlay -->
      <div class="absolute bottom-4 right-4 w-80 bg-black/90 border border-gray-700 rounded-lg p-4 backdrop-blur-sm">
        <h3 class="text-white font-semibold mb-4 flex items-center gap-2">
          <div class="w-2 h-2 bg-{isComplete ? 'green' : 'green'}-400 rounded-full {isComplete ? '' : 'animate-pulse'}"></div>
          Deployment Status
        </h3>
        <div class="space-y-2 h-32 overflow-y-auto">
          {#each config.statusUpdates as status, i}
            {#if currentTime >= status.delay}
              <div
                class="flex items-center gap-3 p-2 bg-gray-900/50 border border-gray-700 rounded text-xs"
                in:fly={{ y: 10, duration: 300 }}
              >
                <div class="w-2 h-2 bg-{status.color}-400 rounded-full"></div>
                <span class="text-gray-300">{status.message}</span>
                <span class="text-{status.color}-400 ml-auto">{status.status}</span>
              </div>
            {/if}
          {/each}
        </div>
      </div>

      <!-- Progress Bar - Top Overlay -->
      <div class="absolute top-20 left-1/2 transform -translate-x-1/2 w-96 bg-black/90 border border-gray-700 rounded-lg p-4 backdrop-blur-sm">
        <div class="flex justify-between items-center mb-2">
          <span class="text-gray-400 text-sm">Deployment Progress</span>
          <span class="text-cyan-400 text-sm">
            {Math.round((currentTime / config.duration) * 100)}%
          </span>
        </div>
        <div class="w-full bg-gray-800 border border-gray-700 rounded-full h-3">
          <div
            class="bg-gradient-to-r from-cyan-500 to-cyan-400 h-3 rounded-full transition-all duration-100"
            style="width: {Math.min((currentTime / config.duration) * 100, 100)}%"
          />
        </div>
        
        <!-- Completion Controls -->
        {#if isComplete}
          <div class="mt-3 text-center">
            <div class="mb-2">
              <div class="inline-flex items-center gap-2 px-3 py-1 bg-green-900/30 border border-green-600/40 rounded text-xs">
                <div class="w-2 h-2 bg-green-400 rounded-full"></div>
                <span class="text-green-400 font-medium">Complete!</span>
              </div>
            </div>
            <div class="flex justify-center gap-2">
              <button
                class="px-3 py-1 bg-blue-600 hover:bg-blue-700 text-white rounded text-xs transition-colors flex items-center gap-1"
                on:click={() => {
                  // Reset animation logic here
                  currentTime = 0;
                  terminalLineIndex = 0;
                  isComplete = false;
                  particles = [];
                }}
              >
                ↻ Replay
              </button>
              <button
                class="px-3 py-1 bg-gray-600 hover:bg-gray-700 text-white rounded text-xs transition-colors flex items-center gap-1"
                on:click={onComplete}
              >
                ✕ Close
              </button>
            </div>
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>
{/if}