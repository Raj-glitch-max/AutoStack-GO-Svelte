<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert, Button, Input, Label, Select, Textarea } from "flowbite-svelte";
  import { ArrowRight, CheckCircle2, Circle, AlertCircle } from "lucide-svelte";
  import toast from "svelte-french-toast";

  let loading = true;
  let error = "";
  let accounts: any[] = [];
  let projects: any[] = [];
  let cost: any = null;
  let estimating = false;

  // Wizard state
  let step = 1;
  let selectedAccountID = "";
  let selectedProjectID = "";
  let image = "nginx:latest";
  let cpu = 1.0;
  let memoryMB = 512;
  let replicas = 2;

  async function load() {
    loading = true;
    try {
      const [accs, projRes] = await Promise.all([
        client.send("/api/v1/cloud-accounts", { method: "GET" }),
        client.collection("projects").getList(1, 50, { sort: "-created" }),
      ]);
      accounts = (Array.isArray(accs) ? accs : []).filter((a: any) => a.status === "active");
      projects = projRes?.items ?? [];
    } catch (e: any) {
      error = e?.message ?? "Failed to load deployment data";
    } finally {
      loading = false;
    }
  }

  onMount(load);

  $: accountOptions = accounts.map((a: any) => ({
    value: a.id,
    name: `${a.name} (${a.provider}, ${a.region || "no region"})`,
  }));
  $: projectOptions = projects.map((p: any) => ({ value: p.id, name: p.name }));

  $: selectedAccount = accounts.find((a) => a.id === selectedAccountID);

  async function estimateCost() {
    if (!selectedAccountID) { toast.error("Pick a cloud account"); return; }
    estimating = true;
    cost = null;
    try {
      cost = await client.send("/api/v1/cost/estimate", {
        method: "POST",
        body: JSON.stringify({
          cloud_account_id: selectedAccountID,
          spec: {
            Image: { Repository: image.split(":")[0], Tag: image.split(":")[1] || "latest" },
            Compute: { CPURequestVCPU: cpu, MemoryRequestMB: memoryMB },
            Scale: { MinReplicas: replicas, MaxReplicas: replicas },
          },
        }),
      });
      if (cost?.error) {
        toast.error(`Cost estimate not available: ${cost.error}`);
      }
    } catch (e: any) {
      const msg = e?.response?.error ?? e?.message ?? "Estimate failed";
      // The cost-estimate endpoint requires the cloud account to be reachable
      // with valid credentials — surface this honestly rather than faking it.
      toast.error(msg);
    } finally {
      estimating = false;
    }
  }

  function stepIcon(n: number) {
    if (step > n) return CheckCircle2;
    if (step === n) return Circle;
    return Circle;
  }
</script>

<div class="max-w-4xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div>
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">First Cloud Deployment</h1>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      Guided flow for deploying a containerized app to a connected cloud provider.
    </p>
  </div>

  <!-- Honest preview banner -->
  <Alert color="yellow">
    <div class="flex items-start gap-2">
      <AlertCircle class="w-5 h-5 mt-0.5 shrink-0" />
      <div class="text-xs">
        <p class="font-semibold">Preview &mdash; not all steps are wired to a real provider deploy yet.</p>
        <p class="mt-1">
          Steps 1–3 are wired: list/pick accounts, projects, and request a cost estimate via the
          live <code>/api/v1/cost/estimate</code> endpoint. Step 4 (provision) is intentionally
          not auto-fired from this UI — wiring it requires the deployment_targets writer flow
          which is the next product pass. To deploy today, use the K8s/operator path under
          <a href="/app" class="underline">Projects</a>.
        </p>
      </div>
    </div>
  </Alert>

  {#if loading}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else}
    <!-- step indicator -->
    <div class="flex items-center gap-4 text-xs flex-wrap">
      {#each [
        { n: 1, label: "Cloud account" },
        { n: 2, label: "Project + image" },
        { n: 3, label: "Cost estimate" },
        { n: 4, label: "Provision (preview)" },
      ] as s}
        <div class="flex items-center gap-1.5 {step >= s.n ? 'text-primary-600 dark:text-primary-400 font-medium' : 'text-gray-400'}">
          <svelte:component this={stepIcon(s.n)} class="w-4 h-4" />
          {s.n}. {s.label}
        </div>
      {/each}
    </div>

    <!-- Step 1: Pick cloud account -->
    <Card padding="md">
      <h2 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">1. Choose a cloud account</h2>
      {#if accounts.length === 0}
        <Alert color="yellow">
          No <code>active</code> cloud accounts found. <a href="/app/cloud/accounts" class="underline">Connect one first</a>.
        </Alert>
      {:else}
        <Select bind:value={selectedAccountID} items={accountOptions} placeholder="Select an account…" size="sm" />
        {#if selectedAccount}
          <p class="mt-2 text-xs text-gray-500">
            Provider: <Badge color="blue" class="text-xs">{selectedAccount.provider}</Badge>
            &middot; Region: <code>{selectedAccount.region || "—"}</code>
            &middot; Status: <Badge color="green" class="text-xs">{selectedAccount.status}</Badge>
          </p>
        {/if}
        {#if selectedAccountID && step < 2}
          <Button color="primary" size="sm" class="mt-3" on:click={() => (step = 2)}>
            Next: project &amp; image
            <ArrowRight class="w-4 h-4 ml-1.5" />
          </Button>
        {/if}
      {/if}
    </Card>

    <!-- Step 2: Project + image -->
    {#if step >= 2}
      <Card padding="md">
        <h2 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">2. Pick project &amp; container image</h2>
        <div class="space-y-3">
          <Label class="space-y-1.5">
            <span class="text-xs">Project</span>
            {#if projects.length === 0}
              <Alert color="yellow">No projects yet. <a href="/app" class="underline">Create one</a>.</Alert>
            {:else}
              <Select bind:value={selectedProjectID} items={projectOptions} placeholder="Select project…" size="sm" />
            {/if}
          </Label>
          <Label class="space-y-1.5">
            <span class="text-xs">Container image</span>
            <Input bind:value={image} placeholder="nginx:latest" />
          </Label>
          <div class="grid grid-cols-3 gap-3">
            <Label class="space-y-1.5">
              <span class="text-xs">vCPU</span>
              <Input type="number" bind:value={cpu} step="0.25" min="0.25" />
            </Label>
            <Label class="space-y-1.5">
              <span class="text-xs">Memory (MB)</span>
              <Input type="number" bind:value={memoryMB} step="128" min="128" />
            </Label>
            <Label class="space-y-1.5">
              <span class="text-xs">Replicas</span>
              <Input type="number" bind:value={replicas} step="1" min="1" max="20" />
            </Label>
          </div>
          {#if step === 2 && selectedProjectID && image}
            <Button color="primary" size="sm" on:click={() => (step = 3)}>
              Next: cost estimate
              <ArrowRight class="w-4 h-4 ml-1.5" />
            </Button>
          {/if}
        </div>
      </Card>
    {/if}

    <!-- Step 3: Cost estimate -->
    {#if step >= 3}
      <Card padding="md">
        <h2 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">3. Cost estimate</h2>
        <p class="text-xs text-gray-500 mb-3">
          Calls the provider-aware <code>/api/v1/cost/estimate</code> endpoint. Estimates are
          honest ranges &mdash; never guarantees.
        </p>
        <Button color="alternative" size="sm" on:click={estimateCost} disabled={estimating}>
          {estimating ? "Estimating…" : "Request estimate"}
        </Button>
        {#if cost && !cost.error}
          <div class="mt-3 rounded border border-gray-200 dark:border-gray-700 p-3 text-xs">
            <pre class="whitespace-pre-wrap font-mono">{JSON.stringify(cost, null, 2)}</pre>
          </div>
        {/if}
        {#if step === 3 && cost && !cost.error}
          <Button color="primary" size="sm" class="mt-3" on:click={() => (step = 4)}>
            Next: provision (preview)
            <ArrowRight class="w-4 h-4 ml-1.5" />
          </Button>
        {/if}
      </Card>
    {/if}

    <!-- Step 4: Provision (intentionally not wired in this release) -->
    {#if step >= 4}
      <Card padding="md">
        <h2 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">
          4. Provision <Badge color="yellow" class="text-xs ml-2">preview</Badge>
        </h2>
        <p class="text-xs text-gray-600 dark:text-gray-400">
          The provider call to create a deployment_target is intentionally not fired from this
          UI in this release. The reason: provisioning requires writing to
          <code>deployment_targets</code> with the right network / registry / DNS scaffolding,
          and we want to ship that as one coherent wizard rather than a half-done button that
          could leave you with a stuck cloud resource.
        </p>
        <p class="mt-2 text-xs text-gray-600 dark:text-gray-400">
          To deploy a container today: use the Kubernetes operator path from your
          <a href="/app" class="underline text-primary-600">Projects</a> page.
        </p>
      </Card>
    {/if}
  {/if}
</div>
