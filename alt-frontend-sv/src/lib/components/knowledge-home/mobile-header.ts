export type ServiceQuality = "full" | "degraded" | "fallback";

export function getQualityDotClass(quality: ServiceQuality): string {
	switch (quality) {
		case "full":
			return "bg-[var(--badge-green-text)]";
		case "degraded":
			return "bg-[var(--badge-amber-text)] animate-pulse";
		case "fallback":
			return "bg-[var(--badge-orange-text)]";
	}
}

export function getQualityLabel(quality: ServiceQuality): string {
	return `Service status: ${quality}`;
}
