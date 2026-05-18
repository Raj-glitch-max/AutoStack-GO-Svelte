<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert, Button, Input, Label, Modal, Select, Textarea } from "flowbite-svelte";
  import { Plus, RefreshCw } from "lucide-svelte";
  import toast from "svelte-french-toast";

  let loading = true;
  let error = "";
  let accounts: any[] = [];

  let modalOpen = false;
  let creating = false;
  let validating = "";

  // form state
  let name = "";
  let provider = "aws";
  let region = "us-east-1";
  let credentialsJSON = "";

  const providerOptions = [
    { value: "aws",   name: "AWS (ECS Fargate)" },
    { value: "gcp",   name: "Google Cloud (Cloud Run)" },
    { value: "azure", name: "Azure (Container Apps)" },
  ];

  const credentialHints: Record<string, string> = {
    aws: `{"access_key_id":"AKIA…","secret_access_key":"…"}`,
    gcp: `{"service_account_json":"<paste full SA JSON>"}`,
    azure: `{"client_id":"…","client_secret":"…","tenant_id":"…","subscription_id":"…"}`,
  };

  async function fetchAccounts() {
    loading = true;
    error = "";
    try {
      const accs = await client.send("/api/v1/cloud-accounts", { method: "GET" });
      accounts = Array.isArray(accs) ? accs : [];
    } catch (e: any) {
      error = e?.message ?? "Failed to load cloud accounts";
    } finally {
      loading = false;
    }
  }

  onMount(fetchAccounts);

  async function createAccount() {
    if (!name.trim()) { toast.error("Name is required"); return; }
    if (!credentialsJSON.trim()) { toast.error("Credentials JSON is required"); return; }
    try {
      JSON.parse(credentialsJSON);
    } catch {
      toast.error("Credentials field must be valid JSON");
      return;
    }
    creating = true;
    try {
      await client.send("/api/v1/cloud-accounts", {
        method: "POST",
        body: JSON.stringify({
          name: name.trim(),
          provider,
          region: region.trim(),
          credentials_json: credentialsJSON,
        }),
      });
      toast.success(`Account "${name}" created — validating against ${provider}…`);
      modalOpen = false;
      name = ""; credentialsJSON = "";
      await fetchAccounts();
    } catch (e: any) {
      const msg = e?.response?.error ?? e?.message ?? "Create failed";
      toast.error(msg);
    } finally {
      creating = false;
    }
  }

  async function revalidate(id: string) {
    validating = id;
    try {
      await client.send(`/api/v1/cloud-accounts/${id}/validate`, { method: "POST" });
      toast.success("Re-validated");
      await fetchAccounts();
    } catch (e: any) {
      toast.error(e?.message ?? "Validation failed");
    } finally {
      validating = "";
    }
  }

  function statusColor(s: string): string {
    switch (s) {
      case "active": return "green";
      case "validating": return "yellow";
      case "error": return "red";
      case "revoked": return "gray";
      default: return "gray";
    }
  }

  function formatTs(ts: string): string {
    if (!ts) return "—";
    try { return new Date(ts).toLocaleString(); } catch { return ts; }
  }
</script>

<div class="max-w-5xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div class="flex items-start justify-between gap-3">
    <div>
      <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Cloud Accounts</h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        Connect cloud provider accounts. Credentials are encrypted at rest (AES-256-GCM) and
        validated against the provider's identity API.
      </p>
    </div>
    <div class="flex gap-2">
      <Button color="alternative" size="sm" on:click={fetchAccounts} disabled={loading}>
        <RefreshCw class="w-4 h-4 mr-1.5 {loading ? 'animate-spin' : ''}" />
        Refresh
      </Button>
      <Button color="primary" size="sm" on:click={() => (modalOpen = true)}>
        <Plus class="w-4 h-4 mr-1.5" />
        Connect Account
      </Button>
    </div>
  </div>

  {#if loading}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if accounts.length === 0}
    <Card padding="lg" class="text-center">
      <p class="text-base font-semibold text-gray-800 dark:text-gray-200">No accounts connected yet</p>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        Connect your first AWS, GCP, or Azure account to start deploying.
      </p>
      <Button color="primary" size="sm" class="mt-4" on:click={() => (modalOpen = true)}>
        <Plus class="w-4 h-4 mr-1.5" />
        Connect Account
      </Button>
    </Card>
  {:else}
    <div class="grid gap-3 sm:grid-cols-2">
      {#each accounts as acc}
        <Card padding="md">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2 flex-wrap">
                <p class="text-sm font-semibold text-gray-800 dark:text-gray-200 truncate">{acc.name}</p>
                <Badge color={statusColor(acc.status)} class="text-xs">{acc.status}</Badge>
              </div>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                {acc.provider} &middot; {acc.region || "no region"}
              </p>
              {#if acc.last_validated}
                <p class="mt-0.5 text-xs text-gray-400">
                  last validated: {formatTs(acc.last_validated)}
                </p>
              {/if}
              {#if acc.validation_error}
                <div class="mt-2 rounded border border-red-200 bg-red-50 dark:bg-red-900/20 p-2">
                  <p class="text-xs text-red-700 dark:text-red-300 break-all">{acc.validation_error}</p>
                </div>
              {/if}
            </div>
          </div>
          <div class="mt-3 flex gap-2 text-xs">
            <Button color="alternative" size="xs" on:click={() => revalidate(acc.id)} disabled={validating === acc.id}>
              <RefreshCw class="w-3 h-3 mr-1 {validating === acc.id ? 'animate-spin' : ''}" />
              {validating === acc.id ? "Validating…" : "Re-validate"}
            </Button>
          </div>
        </Card>
      {/each}
    </div>
  {/if}

  <div class="rounded-md border border-blue-200 bg-blue-50 dark:bg-blue-900/20 p-4 space-y-1">
    <p class="text-xs font-semibold uppercase tracking-wide text-blue-700 dark:text-blue-400">
      How credentials are handled
    </p>
    <p class="text-xs text-blue-800 dark:text-blue-300">
      Credentials are AES-256-GCM encrypted at write time using the platform's encryption key
      (fingerprinted in startup logs). They are never logged, never returned through the API,
      and never persisted in plaintext. Validation makes a live identity call to the provider
      (AWS STS GetCallerIdentity, GCP IAM, Azure ARM). A failure returns the provider's actual
      error message — no fake successes.
    </p>
  </div>
</div>

<Modal bind:open={modalOpen} size="md" title="Connect a cloud account" autoclose={false}>
  <div class="space-y-4">
    <Label class="space-y-1.5">
      <span>Display name</span>
      <Input bind:value={name} placeholder="e.g. AWS Prod us-east-1" required />
    </Label>

    <Label class="space-y-1.5">
      <span>Provider</span>
      <Select bind:value={provider} items={providerOptions} size="sm" />
    </Label>

    <Label class="space-y-1.5">
      <span>Default region</span>
      <Input bind:value={region} placeholder="us-east-1" />
    </Label>

    <Label class="space-y-1.5">
      <span>Credentials (JSON)</span>
      <Textarea
        bind:value={credentialsJSON}
        rows={5}
        placeholder={credentialHints[provider]}
        class="font-mono text-xs"
      />
      <p class="text-xs text-gray-500 dark:text-gray-400">
        Paste provider credentials as JSON. Encrypted before persistence; never logged.
      </p>
    </Label>

    <div class="rounded border border-yellow-200 bg-yellow-50 dark:bg-yellow-900/20 p-2.5">
      <p class="text-xs text-yellow-800 dark:text-yellow-300">
        Validation runs immediately. Invalid credentials produce a real provider error (e.g.
        "InvalidClientTokenId") and the account is saved with <code>status=error</code>.
      </p>
    </div>
  </div>

  <svelte:fragment slot="footer">
    <div class="flex gap-2 justify-end w-full">
      <Button color="alternative" on:click={() => (modalOpen = false)} disabled={creating}>Cancel</Button>
      <Button color="primary" on:click={createAccount} disabled={creating}>
        {creating ? "Connecting…" : "Connect & Validate"}
      </Button>
    </div>
  </svelte:fragment>
</Modal>
