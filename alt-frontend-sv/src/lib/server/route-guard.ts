/**
 * Route guard - public route classification logic extracted from hooks.server.ts
 */

const PUBLIC_ROUTES = [
	/\/auth(\/|$)/,
	/\/health(\/|$)/,
	/\/login(\/|$)/,
	/\/register(\/|$)/,
	/\/logout(\/|$)/,
	/\/recovery(\/|$)/,
	/\/verification(\/|$)/,
	/\/error(\/|$)/,
	/\/public\/landing(\/|$)/,
	/\/landing$/,
	/\/favicon\.ico$/,
	/\/icon\.svg$/,
	/\/test(\/|$)/,
];

export function isPublicRoute(pathname: string): boolean {
	return PUBLIC_ROUTES.some((pattern) => pattern.test(pathname));
}

export function isApiRoute(pathname: string): boolean {
	return pathname.startsWith("/sv/api/") || pathname.startsWith("/api/");
}

export function isStreamEndpoint(pathname: string): boolean {
	return pathname.includes("/stream") || pathname.includes("/sse");
}
