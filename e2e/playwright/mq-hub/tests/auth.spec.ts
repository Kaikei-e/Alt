import { test, expect } from "../src/fixtures.js";
import { env, Procedure } from "../src/env.js";
import { callUnary, expectUnaryError, ConnectCode } from "../../_shared/connect.js";

test("missing and invalid bearer reject a real MQ procedure", async ({ playwright, api }) => {
	const anonymous = await playwright.request.newContext({ baseURL: env.baseURL });
	try {
		await expectUnaryError(anonymous, Procedure.publishBatch, { events: [] }, ConnectCode.unauthenticated);
		await expectUnaryError(anonymous, Procedure.publishBatch, { events: [] }, ConnectCode.unauthenticated, { headers: { Authorization: "Bearer invalid-fixture" } });
		expect((await callUnary(api, Procedure.publishBatch, { events: [] })).status()).toBe(200);
	} finally {
		await anonymous.dispose();
	}
});
