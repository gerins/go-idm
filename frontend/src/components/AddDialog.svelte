<script lang="ts">
  import Modal from './Modal.svelte'
  import AddForm from './AddForm.svelte'
  import { ui } from '../lib/ui.svelte'
</script>

<Modal bind:open={ui.addOpen} title="Add download" width={540}>
  <AddForm onclose={() => (ui.addOpen = false)} />
</Modal>
