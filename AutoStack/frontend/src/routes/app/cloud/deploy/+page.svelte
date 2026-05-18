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
  let containerPort = 80;
  let deployName = "";
  let deploying = false;
  let lastDeployResult: any = null;

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

  async function deployToCloud() {
    if (!selectedAccountID || !image.trim()) {
      toast.error("Pick an account and provide an image");
      return;
    }
    if (!deployName.trim()) {
      deployName = `cloud-deploy-${Date.now()}`;
    }
    deploying = true;
    lastDeployResult = null;
    try {
      const result = await client.send("/api/v1/cloud-deployments", {
        method: "POST",
        body: JSON.stringify({
          cloud_account_id: selectedAccountID,
          name: deployName.trim(),
          image: image.trim(),
          cpu_vcpu: Number(cpu) || 1,
          memory_mb: Number(memoryMB) || 512,
          replicas: Number(replicas) || 1,
          container_port: Number(containerPort) || 80,
        }),
      });
      lastDeployResult = result;
      toast.success(`Cloud deployment queued (id=${(result as any).id?.slice(0, 8) ?? "?"}…)`);
    } catch (e: any) {
      // The provider's real error message comes back here — surface it verbatim.
      const msg = e?.response?.error ?? e?.message ?? "Deploy failed";
      lastDeployResult = { status: "error", error: msg };
      toast.error(msg);
    } finally {
      deploying = false;
    }
  }

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

  <!-- Honest cloud-execution banner -->
  <Alert color="yellow">
    <div class="flex items-start gap-2">
      <AlertCircle class="w-5 h-5 mt-0.5 shrink-0" />
      <div class="text-xs">
        <p class="font-semibold">Cloud deployment calls REAL provider SDKs (AWS ECS / Cloud Run / Azure ACA).</p>
        <p class="mt-1">
          Whether a deployment actually provisions depends entirely on the cloud_accounts row's credentials.
          With valid credentials, this will create a real cloud resource and bill you.
          With demo or invalid credentials, the provider returns its real error
          (e.g. <code>InvalidClientTokenId</code>, <code>credentials: unsupported file type</code>)
          and the deployment row is saved with <code>status=error</code>. No fake success.
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

    <!-- Step 4: Provision — real provider call -->
    {#if step >= 4}
      <Card padding="md">
        <h2 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">
          4. Provision
        </h2>
        <div class="space-y-3">
          <Label class="space-y-1.5">
            <span class="text-xs">Deployment name (auto-generated if blank)</span>
            <Input bind:value={deployName} placeholder="e.g. customer-portal-prod" />
          </Label>
          <Label class="space-y-1.5">
            <span class="text-xs">Container port</span>
            <Input type="number" bind:value={containerPort} min="1" max="65535" />
          </Label>
          <div class="rounded border border-orange-200 bg-orange-50 dark:bg-orange-900/20 p-3 text-xs">
            <strong>This calls a REAL provider API:</strong>
            <ul class="mt-1 space-y-0.5 list-disc list-inside">
              <li><code>aws</code> → ECS RegisterTaskDefinition + CreateService / UpdateService</li>
              <li><code>gcp</code> → Cloud Run admin API CreateService</li>
              <li><code>azure</code> → ACA admin API (not yet contract-tested)</li>
            </ul>
            <p class="mt-1">If your credentials are real, this will create cloud resources you will be billed for.</p>
          </div>
          <Button color="primary" on:click={deployToCloud} disabled={deploying || !selectedAccountID}>
            {deploying ? "Calling provider…" : "Deploy now"}
          </Button>

          {#if lastDeployResult}
            <div class="rounded border p-3 text-xs space-y-1
              {lastDeployResult.status === 'provisioning' ? 'border-blue-200 bg-blue-50 dark:bg-blue-900/20' :
               lastDeployResult.status === 'error' ? 'border-red-200 bg-red-50 dark:bg-red-900/20' :
               'border-gray-200 bg-gray-50 dark:bg-gray-800'}">
              <div class="flex items-center gap-2 flex-wrap">
                <Badge color={lastDeployResult.status === 'provisioning' ? 'blue' : lastDeployResult.status === 'error' ? 'red' : 'gray'}>
                  {lastDeployResult.status ?? "unknown"}
                </Badge>
                {#if lastDeployResult.id}
                  <code>{lastDeployResult.id}</code>
                {/if}
              </div>
              {#if lastDeployResult.endpoint_url}
                <p>endpoint: <a href={lastDeployResult.endpoint_url} target="_blank" rel="noopener" class="underline">{lastDeployResult.endpoint_url}</a></p>
              {/if}
              {#if lastDeployResult.external_id}
                <p>external id: <code>{lastDeployResult.external_id}</code></p>
              {/if}
              {#if lastDeployResult.message}
                <p>{lastDeployResult.message}</p>
              {/if}
              {#if lastDeployResult.error}
                <p class="text-red-700 dark:text-red-300 font-mono">{lastDeployResult.error}</p>
              {/if}
            </div>
          {/if}
        </div>
      </Card>
    {/if}
  {/if}
</div>
