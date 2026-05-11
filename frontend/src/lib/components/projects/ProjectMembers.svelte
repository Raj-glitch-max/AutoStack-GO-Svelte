<script lang="ts">
  import { pb } from '$lib/pocketbase';
  import { onMount } from 'svelte';
  import { Button, Modal, Label, Input, Select, Badge, Table, TableBody, TableBodyCell, TableBodyRow, TableHead, TableHeadCell } from 'flowbite-svelte';
  import { Plus, Trash2, UserCircle } from 'lucide-svelte';

  export let projectId: string;
  export let userRole: string = 'viewer';

  let members: any[] = [];
  let loading = true;
  let showInviteModal = false;
  let inviteEmail = '';
  let inviteRole = 'viewer';
  let inviting = false;

  const roles = [
    { value: 'owner', name: 'Owner' },
    { value: 'deployer', name: 'Deployer' },
    { value: 'viewer', name: 'Viewer' }
  ];

  onMount(async () => {
    await fetchMembers();
  });

  async function fetchMembers() {
    loading = true;
    try {
      members = await pb.collection('project_members').getFullList({
        filter: `project = "${projectId}"`,
        expand: 'user'
      });
    } catch (err) {
      console.error('Error fetching members:', err);
    } finally {
      loading = false;
    }
  }

  async function inviteMember() {
    inviting = true;
    try {
      // 1. Find user by email
      const users = await pb.collection('users').getList(1, 1, {
        filter: `email = "${inviteEmail}"`
      });

      if (users.items.length === 0) {
        alert('User not found with this email.');
        return;
      }

      const targetUser = users.items[0];

      // 2. Create membership
      await pb.collection('project_members').create({
        project: projectId,
        user: targetUser.id,
        role: inviteRole
      });

      showInviteModal = false;
      inviteEmail = '';
      await fetchMembers();
    } catch (err) {
      console.error('Error inviting member:', err);
      alert('Failed to invite member. They might already be part of the project.');
    } finally {
      inviting = false;
    }
  }

  async function updateRole(memberId: string, newRole: string) {
    try {
      await pb.collection('project_members').update(memberId, {
        role: newRole
      });
      await fetchMembers();
    } catch (err) {
      console.error('Error updating role:', err);
      alert('Failed to update role.');
    }
  }

  async function removeMember(memberId: string) {
    if (!confirm('Are you sure you want to remove this member?')) return;

    try {
      await pb.collection('project_members').delete(memberId);
      await fetchMembers();
    } catch (err) {
      console.error('Error removing member:', err);
      alert('Failed to remove member.');
    }
  }
</script>

<div class="space-y-4">
  <div class="flex justify-between items-center">
    <div>
      <h3 class="text-xl font-bold text-white">Project Members</h3>
      <p class="text-gray-400 text-sm">Manage who has access to this project.</p>
    </div>
    {#if userRole === 'owner'}
      <Button color="alternative" on:click={() => showInviteModal = true}>
        <Plus class="w-4 h-4 mr-2" />
        Invite Member
      </Button>
    {/if}
  </div>

  <div class="bg-gray-800 rounded-lg overflow-hidden border border-gray-700">
    <Table hoverable={true}>
      <TableHead class="bg-gray-700 text-gray-300">
        <TableHeadCell>User</TableHeadCell>
        <TableHeadCell>Role</TableHeadCell>
        <TableHeadCell>Actions</TableHeadCell>
      </TableHead>
      <TableBody>
        {#each members as member}
          <TableBodyRow class="border-gray-700">
            <TableBodyCell class="flex items-center space-x-3">
              <UserCircle class="w-8 h-8 text-gray-500" />
              <div>
                <div class="text-white font-medium">{member.expand?.user?.name || member.expand?.user?.username}</div>
                <div class="text-gray-400 text-xs">{member.expand?.user?.email}</div>
              </div>
            </TableBodyCell>
            <TableBodyCell>
              {#if userRole === 'owner' && member.user !== pb.authStore.model?.id}
                <Select
                  size="sm"
                  class="bg-gray-700 border-gray-600 text-white"
                  items={roles}
                  bind:value={member.role}
                  on:change={() => updateRole(member.id, member.role)}
                />
              {:else}
                <Badge color={member.role === 'owner' ? 'indigo' : member.role === 'deployer' ? 'green' : 'dark'}>
                  {member.role.toUpperCase()}
                </Badge>
              {/if}
            </TableBodyCell>
            <TableBodyCell>
              {#if userRole === 'owner' && member.user !== pb.authStore.model?.id}
                <Button color="red" size="xs" on:click={() => removeMember(member.id)}>
                  <Trash2 class="w-4 h-4" />
                </Button>
              {/if}
            </TableBodyCell>
          </TableBodyRow>
        {/each}
        {#if members.length === 0 && !loading}
          <TableBodyRow>
            <TableBodyCell colspan="3" class="text-center py-8 text-gray-500">
              No members found.
            </TableBodyCell>
          </TableBodyRow>
        {/if}
      </TableBody>
    </Table>
  </div>
</div>

<Modal title="Invite New Member" bind:open={showInviteModal} autoclose={false} size="sm" class="bg-gray-800">
  <div class="space-y-4">
    <div>
      <Label for="email" class="text-white mb-2">Email Address</Label>
      <Input id="email" type="email" placeholder="user@example.com" bind:value={inviteEmail} class="bg-gray-700 border-gray-600 text-white" />
    </div>
    <div>
      <Label for="role" class="text-white mb-2">Project Role</Label>
      <Select id="role" items={roles.filter(r => r.value !== 'owner')} bind:value={inviteRole} class="bg-gray-700 border-gray-600 text-white" />
    </div>
    <div class="flex justify-end space-x-3 mt-6">
      <Button color="alternative" on:click={() => showInviteModal = false}>Cancel</Button>
      <Button color="blue" on:click={inviteMember} disabled={inviting || !inviteEmail}>
        {inviting ? 'Inviting...' : 'Send Invite'}
      </Button>
    </div>
  </div>
</Modal>
