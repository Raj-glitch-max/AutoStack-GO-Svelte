<script lang="ts">
  import { Button, Input, Label, Select, Fileupload } from "flowbite-svelte";
  import { Key, Upload, CheckCircle, AlertCircle } from "lucide-svelte";
  import toast from "svelte-french-toast";
  
  let accessKeyId = '';
  let secretAccessKey = '';
  let sessionToken = '';
  let region = 'us-east-1';
  let isValidating = false;
  let isValid = false;
  let validationError = '';
  let validatedAccountId = '';
  let validatedUserArn = '';
  
  const regions = [
    { value: 'us-east-1', name: 'US East (N. Virginia)' },
    { value: 'us-east-2', name: 'US East (Ohio)' },
    { value: 'us-west-1', name: 'US West (N. California)' },
    { value: 'us-west-2', name: 'US West (Oregon)' },
    { value: 'eu-west-1', name: 'Europe (Ireland)' },
    { value: 'eu-central-1', name: 'Europe (Frankfurt)' },
    { value: 'ap-southeast-1', name: 'Asia Pacific (Singapore)' },
    { value: 'ap-northeast-1', name: 'Asia Pacific (Tokyo)' }
  ];
  
  async function handleFileUpload(event: Event) {
    const target = event.target as HTMLInputElement;
    const file = target.files?.[0];
    
    if (!file) return;
    
    if (!file.name.endsWith('.csv')) {
      toast.error('Please upload a CSV file');
      return;
    }
    
    try {
      const text = await file.text();
      const lines = text.split('\n');
      
      if (lines.length < 2) {
        toast.error('Invalid CSV format');
        return;
      }
      
      // Parse CSV header
      const headers = lines[0].split(',').map(h => h.trim().toLowerCase());
      const values = lines[1].split(',').map(v => v.trim());
      
      // Find column indices
      const accessKeyIndex = headers.findIndex(h => 
        h.includes('access key') || h.includes('accesskey')
      );
      const secretKeyIndex = headers.findIndex(h => 
        h.includes('secret') || h.includes('secretkey')
      );
      const sessionTokenIndex = headers.findIndex(h => 
        h.includes('session') || h.includes('token')
      );
      
      if (accessKeyIndex === -1 || secretKeyIndex === -1) {
        toast.error('CSV must contain Access Key ID and Secret Access Key columns');
        return;
      }
      
      // Extract values
      accessKeyId = values[accessKeyIndex] || '';
      secretAccessKey = values[secretKeyIndex] || '';
      if (sessionTokenIndex !== -1) {
        sessionToken = values[sessionTokenIndex] || '';
      }
      
      toast.success('Credentials loaded from CSV');
      
    } catch (error) {
      console.error('Error parsing CSV:', error);
      toast.error('Failed to parse CSV file');
    }
  }
  
  async function validateCredentials() {
    if (!accessKeyId.trim() || !secretAccessKey.trim()) {
      toast.error('Please provide Access Key ID and Secret Access Key');
      return;
    }
    
    isValidating = true;
    validationError = '';
    
    try {
      const response = await fetch('/api/aws/credentials/validate', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          accessKeyId: accessKeyId.trim(),
          secretAccessKey: secretAccessKey.trim(),
          sessionToken: sessionToken.trim(),
          region
        })
      });
      
      const result = await response.json();
      
      if (response.ok && result.valid) {
        isValid = true;
        validatedAccountId = result.accountId;
        validatedUserArn = result.userArn;
        toast.success('AWS credentials validated successfully!');
      } else {
        isValid = false;
        validatedAccountId = '';
        validatedUserArn = '';
        validationError = result.error || 'Invalid credentials';
        toast.error('Credential validation failed');
      }
      
    } catch (error) {
      console.error('Validation error:', error);
      isValid = false;
      validationError = 'Failed to validate credentials';
      toast.error('Failed to validate credentials');
    } finally {
      isValidating = false;
    }
  }
  
  async function saveCredentials() {
    if (!isValid) {
      toast.error('Please validate credentials first');
      return;
    }
    
    try {
      const response = await fetch('/api/aws/credentials', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          accessKeyId: accessKeyId.trim(),
          secretAccessKey: secretAccessKey.trim(),
          sessionToken: sessionToken.trim(),
          region
        })
      });
      
      if (response.ok) {
        toast.success('AWS credentials saved successfully!');
        // Clear form
        accessKeyId = '';
        secretAccessKey = '';
        sessionToken = '';
        isValid = false;
      } else {
        toast.error('Failed to save credentials');
      }
      
    } catch (error) {
      console.error('Save error:', error);
      toast.error('Failed to save credentials');
    }
  }
</script>

<div class="max-w-2xl mx-auto p-6">
  <div class="flex items-center space-x-3 mb-6">
    <Key class="w-6 h-6 text-blue-600" />
    <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
      AWS Credentials Setup
    </h2>
  </div>
  
  <div class="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-4 mb-6">
    <p class="text-sm text-blue-800 dark:text-blue-200">
      Your AWS credentials are encrypted and stored securely. They're only used to provision infrastructure on your behalf.
    </p>
  </div>
  
  <!-- File Upload Option -->
  <div class="mb-6">
    <Label class="mb-2 block">
      <span class="flex items-center space-x-2">
        <Upload class="w-4 h-4" />
        <span>Upload rootkey.csv (Optional)</span>
      </span>
    </Label>
    <Fileupload 
      accept=".csv"
      on:change={handleFileUpload}
      class="mb-2"
    />
    <p class="text-sm text-gray-500">
      Upload your AWS credentials CSV file to auto-fill the form below.
    </p>
  </div>
  
  <div class="border-t pt-6">
    <h3 class="text-lg font-medium mb-4">Manual Entry</h3>
    
    <div class="space-y-4">
      <!-- Access Key ID -->
      <Label class="space-y-2">
        <span>Access Key ID *</span>
        <Input
          type="text"
          placeholder="AKIAIOSFODNN7EXAMPLE"
          bind:value={accessKeyId}
          required
        />
      </Label>
      
      <!-- Secret Access Key -->
      <Label class="space-y-2">
        <span>Secret Access Key *</span>
        <Input
          type="password"
          placeholder="wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
          bind:value={secretAccessKey}
          required
        />
      </Label>
      
      <!-- Session Token (Optional) -->
      <Label class="space-y-2">
        <span>Session Token (Optional)</span>
        <Input
          type="password"
          placeholder="For temporary credentials only"
          bind:value={sessionToken}
        />
      </Label>
      
      <!-- Default Region -->
      <Label class="space-y-2">
        <span>Default Region</span>
        <Select bind:value={region}>
          {#each regions as regionOption}
            <option value={regionOption.value}>{regionOption.name}</option>
          {/each}
        </Select>
      </Label>
      
      <!-- Validation Status -->
      {#if validationError}
        <div class="flex items-center space-x-2 text-red-600 dark:text-red-400">
          <AlertCircle class="w-4 h-4" />
          <span class="text-sm">{validationError}</span>
        </div>
      {/if}
      
      {#if isValid}
        <div class="space-y-2">
          <div class="flex items-center space-x-2 text-green-600 dark:text-green-400">
            <CheckCircle class="w-4 h-4" />
            <span class="text-sm font-medium">Credentials validated successfully</span>
          </div>
          <div class="bg-green-50 dark:bg-green-900/10 p-3 rounded-lg text-xs space-y-1">
            <div class="flex justify-between">
              <span class="text-gray-500">Account ID:</span>
              <span class="font-mono font-medium">{validatedAccountId}</span>
            </div>
            <div class="flex flex-col">
              <span class="text-gray-500">User ARN:</span>
              <span class="font-mono break-all">{validatedUserArn}</span>
            </div>
          </div>
        </div>
      {/if}
      
      <!-- Action Buttons -->
      <div class="flex space-x-3 pt-4">
        <Button
          color="alternative"
          on:click={validateCredentials}
          disabled={isValidating || !accessKeyId.trim() || !secretAccessKey.trim()}
        >
          {#if isValidating}
            Validating...
          {:else}
            Validate Credentials
          {/if}
        </Button>
        
        <Button
          color="primary"
          on:click={saveCredentials}
          disabled={!isValid}
        >
          Save Credentials
        </Button>
      </div>
    </div>
  </div>
  
  <!-- Security Notice -->
  <div class="mt-6 p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
    <h4 class="font-medium text-gray-900 dark:text-white mb-2">Security Notice</h4>
    <ul class="text-sm text-gray-600 dark:text-gray-400 space-y-1">
      <li>• Credentials are encrypted using AES-256 encryption</li>
      <li>• Only you can access your credentials</li>
      <li>• Credentials are used only for infrastructure provisioning</li>
      <li>• You can update or delete credentials at any time</li>
    </ul>
  </div>
</div>