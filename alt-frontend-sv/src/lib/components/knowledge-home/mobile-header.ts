export type ServiceQuality = "full" | "degraded" | "fallback";

export function getQualityDotClass(quality: ServiceQuality): string {
	switch (quality) {
		case "full":
			return "bg-[var(--alt-success)]";
		case "degraded":
			return "bg-[var(--alt-warning)] animate-pulse";
		case "fallback":
			return "bg-[var(--alt-error)]";
	}
}

export function getQualityLabel(quality: ServiceQuality): string {
	return `Service status: ${quality}`;
}
