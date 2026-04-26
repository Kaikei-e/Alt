export type PlaneKey = "now" | "continue" | "changed" | "review";

export type LoopPlaneDescriptor = {
	key: PlaneKey;
	label: string;
	caption?: string;
	count: number;
};

export const STACK_ORDER: readonly PlaneKey[] = [
	"now",
	"continue",
	"changed",
	"review",
];
