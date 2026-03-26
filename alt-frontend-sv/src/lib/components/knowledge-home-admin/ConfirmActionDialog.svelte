<script lang="ts">
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "$lib/components/ui/dialog";
import { Button } from "$lib/components/ui/button";

interface Props {
	open: boolean;
	title: string;
	description: string;
	confirmLabel?: string;
	variant?: "default" | "danger";
	onConfirm: () => void;
	onCancel: () => void;
}

let {
	open = false,
	title,
	description,
	confirmLabel = "Confirm",
	variant = "default",
	onConfirm,
	onCancel,
}: Props = $props();
</script>

<Dialog bind:open>
	<DialogContent data-testid="confirm-action-dialog">
		<DialogHeader>
			<DialogTitle>{title}</DialogTitle>
			<DialogDescription>{description}</DialogDescription>
		</DialogHeader>
		<DialogFooter>
			<Button variant="outline" size="sm" onclick={onCancel}>Cancel</Button>
			<Button
				variant={variant === "danger" ? "destructive" : "default"}
				size="sm"
				data-testid="confirm-action-confirm"
				onclick={onConfirm}
			>
				{confirmLabel}
			</Button>
		</DialogFooter>
	</DialogContent>
</Dialog>
