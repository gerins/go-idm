<script lang="ts">
  import Modal from './Modal.svelte'
  import SettingsForm from './SettingsForm.svelte'
  import { ui } from '../lib/ui.svelte'
</script>

<Modal bind:open={ui.settingsOpen} title="Settings" width={560}>
  <SettingsForm onclose={() => (ui.settingsOpen = false)} />
</Modal>
