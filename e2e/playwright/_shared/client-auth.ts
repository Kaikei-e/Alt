/** Native Playwright mTLS; Node's startup NODE_EXTRA_CA_CERTS verifies servers. */
export function clientCertificates(baseURL: string, certPath: string, keyPath: string) {
	return [{ origin: new URL(baseURL).origin, certPath, keyPath }];
}
