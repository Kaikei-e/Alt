type IdentityLike = {
	traits?: Record<string, unknown> | null;
} | null;

export function getUserRole(identity: IdentityLike): "admin" | "user" {
	if (identity?.traits?.role === "admin") {
		return "admin";
	}
	return "user";
}
