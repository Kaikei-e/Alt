import { ConnectCode, callUnary, expectUnaryError } from "../../_shared/connect.js";
import { expectStatus } from "../../_shared/http.js";
import {
	BACKEND_TOKEN_HEADER,
	P,
	expect,
	mintBackendToken,
	test,
} from "../src/fixtures.js";

/**
 * Authentication and authorization enforcement for AcolyteService Connect-RPC surface.
 *
 * UserIdentityInterceptor requires a valid HS256-signed X-Alt-Backend-Token JWT
 * with issuer "auth-hub", audience "alt-backend", and a valid UUID "sub" claim
 * on all business procedures.
 *
 * HealthCheck RPC and REST /health remain unauthenticated by design.
 */
test.describe("backend token authentication @authz", () => {
	test("Connect RPC without X-Alt-Backend-Token is rejected with unauthenticated (401)", async ({
		acolyteAnon,
	}) => {
		const err = await expectUnaryError(
			acolyteAnon,
			P.listReports,
			{},
			ConnectCode.unauthenticated,
		);
		expect(err.message).toContain("missing backend token");
	});

	test("Connect RPC with invalid signature / wrong secret is rejected with unauthenticated (401)", async ({
		acolyteAnon,
	}) => {
		const badToken = mintBackendToken({ secret: "wrong-staging-secret-key-12345" });
		const err = await expectUnaryError(
			acolyteAnon,
			P.listReports,
			{},
			ConnectCode.unauthenticated,
			{ headers: { [BACKEND_TOKEN_HEADER]: badToken } },
		);
		expect(err.message).toContain("invalid backend token");
	});

	test("Connect RPC with expired token is rejected with unauthenticated (401)", async ({
		acolyteAnon,
	}) => {
		const now = Math.floor(Date.now() / 1000);
		const expiredToken = mintBackendToken({
			iat: now - 3600,
			exp: now - 60,
		});
		const err = await expectUnaryError(
			acolyteAnon,
			P.listReports,
			{},
			ConnectCode.unauthenticated,
			{ headers: { [BACKEND_TOKEN_HEADER]: expiredToken } },
		);
		expect(err.message).toContain("backend token has expired");
	});

	test("Connect RPC with invalid issuer is rejected with unauthenticated (401)", async ({
		acolyteAnon,
	}) => {
		const badIssuerToken = mintBackendToken({ issuer: "invalid-issuer" });
		const err = await expectUnaryError(
			acolyteAnon,
			P.listReports,
			{},
			ConnectCode.unauthenticated,
			{ headers: { [BACKEND_TOKEN_HEADER]: badIssuerToken } },
		);
		expect(err.message).toContain("invalid token issuer or audience");
	});

	test("Connect RPC with invalid audience is rejected with unauthenticated (401)", async ({
		acolyteAnon,
	}) => {
		const badAudienceToken = mintBackendToken({ audience: "invalid-audience" });
		const err = await expectUnaryError(
			acolyteAnon,
			P.listReports,
			{},
			ConnectCode.unauthenticated,
			{ headers: { [BACKEND_TOKEN_HEADER]: badAudienceToken } },
		);
		expect(err.message).toContain("invalid token issuer or audience");
	});

	test("Connect RPC with non-UUID sub is rejected with unauthenticated (401)", async ({
		acolyteAnon,
	}) => {
		const nonUuidToken = mintBackendToken({ userId: "not-a-valid-uuid" });
		const err = await expectUnaryError(
			acolyteAnon,
			P.listReports,
			{},
			ConnectCode.unauthenticated,
			{ headers: { [BACKEND_TOKEN_HEADER]: nonUuidToken } },
		);
		expect(err.message).toContain("invalid user id in token");
	});

	test("HealthCheck RPC requires no token even when auth verification is enabled", async ({
		acolyteAnon,
	}) => {
		const response = await callUnary(acolyteAnon, P.healthCheck, {});
		await expectStatus(response, 200);
	});

	test("authenticated Connect RPC with valid HS256 token succeeds (200)", async ({
		acolyte,
	}) => {
		const response = await callUnary(acolyte, P.listReports, { limit: 1 });
		await expectStatus(response, 200);
	});
});
