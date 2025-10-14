import { test as setup, expect } from "@playwright/test";

const authFile = "playwright/.auth/user.json";

setup("authenticate via API", async ({ request, context }) => {
  console.log("[AUTH-SETUP] Starting API-based authentication");

  const mockPort = process.env.PW_MOCK_PORT || "4545";
  const appPort = process.env.PW_APP_PORT || "3010";
  const baseUrl = `http://localhost:${mockPort}`;
  const appBaseUrl = `http://localhost:${appPort}`;

  try {
    // Step 1: Create login flow via API
    console.log("[AUTH-SETUP] Creating login flow via API");
    const returnTo = `${appBaseUrl}/home`;
    const flowResponse = await request.get(`${baseUrl}/self-service/login/browser`, {
      params: { return_to: returnTo },
      maxRedirects: 0,
      failOnStatusCode: false,
    });

    console.log("[AUTH-SETUP] Flow response status:", flowResponse.status());
    console.log("[AUTH-SETUP] Flow response headers:", flowResponse.headers());

    // Extract flow ID from redirect location
    const location = flowResponse.headers()["location"];
    console.log("[AUTH-SETUP] Redirect location:", location);

    if (!location) {
      throw new Error("No redirect location in flow response");
    }

    const flowMatch = location.match(/flow=([a-f0-9-]{32,36})/);
    if (!flowMatch) {
      throw new Error(`Failed to extract flow ID from location: ${location}`);
    }

    const flowId = flowMatch[1];
    console.log(`[AUTH-SETUP] Flow ID: ${flowId}`);

    // Step 2: Get flow details to extract CSRF token
    console.log("[AUTH-SETUP] Getting flow details");
    const flowDetailsResponse = await request.get(`${baseUrl}/self-service/login/flows`, {
      params: { id: flowId },
    });

    if (!flowDetailsResponse.ok()) {
      const errorBody = await flowDetailsResponse.text();
      throw new Error(`Failed to get flow details: ${flowDetailsResponse.status()} - ${errorBody}`);
    }

    const flowData = await flowDetailsResponse.json();
    console.log("[AUTH-SETUP] Flow data received");

    const csrfNode = flowData.ui.nodes.find(
      (node: any) => node.attributes.name === "csrf_token"
    );
    const csrfToken = csrfNode?.attributes.value;

    if (!csrfToken) {
      throw new Error("CSRF token not found in flow");
    }

    console.log("[AUTH-SETUP] CSRF token extracted");

    // Step 3: Submit login form via API
    console.log("[AUTH-SETUP] Submitting login credentials");
    const loginResponse = await request.post(`${baseUrl}/self-service/login?flow=${flowId}`, {
      form: {
        identifier: "test@example.com",
        password: "password123",
        method: "password",
        csrf_token: csrfToken,
      },
    });

    console.log("[AUTH-SETUP] Login response status:", loginResponse.status());

    if (!loginResponse.ok()) {
      const errorBody = await loginResponse.text();
      throw new Error(`Login failed: ${loginResponse.status()} - ${errorBody}`);
    }

    const loginData = await loginResponse.json();
    console.log("[AUTH-SETUP] Login successful");

    // Step 4: Verify session was created
    if (!loginData.session) {
      throw new Error("No session in login response");
    }

    console.log("[AUTH-SETUP] Session created:", loginData.session.id);

    // Step 5: Set session cookie manually
    const sessionCookie = {
      name: "ory_kratos_session",
      value: loginData.session.id,
      domain: "localhost",
      path: "/",
      httpOnly: true,
      secure: false,
      sameSite: "Lax" as const,
    };

    await context.addCookies([sessionCookie]);
    console.log("[AUTH-SETUP] Session cookie set");

    // Step 6: Save authentication state
    await context.storageState({ path: authFile });
    console.log("[AUTH-SETUP] Authentication state saved to", authFile);

    // Verify the auth file was created and has content
    const fs = await import("fs");
    if (!fs.existsSync(authFile)) {
      throw new Error("Auth file was not created");
    }

    const authContent = fs.readFileSync(authFile, "utf-8");
    const authData = JSON.parse(authContent);

    if (!authData.cookies || authData.cookies.length === 0) {
      throw new Error("Auth file has no cookies");
    }

    console.log(`[AUTH-SETUP] ✅ Authentication completed successfully with ${authData.cookies.length} cookie(s)`);
  } catch (error) {
    console.error("[AUTH-SETUP] ❌ Authentication failed:", error);
    throw error;
  }
});
