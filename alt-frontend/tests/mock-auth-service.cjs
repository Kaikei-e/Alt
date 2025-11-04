const http = require("http");
const url = require("url");
const querystring = require("querystring");
const crypto = require("crypto");

const port = process.env.MOCK_AUTH_PORT || 4545;

// In-memory storage for flows
const flows = new Map();
const sessions = new Map();

// Configuration
const config = {
  // No response delay for maximum speed
  responseDelay: 0, // Instant response (reduced from 5ms)
  // Enable detailed logging only when needed
  verbose: false, // Set to true for debugging
  // Flow expiration time (5 minutes for test efficiency)
  flowExpiration: 5 * 60 * 1000, // 5 minutes (reduced from 15)
};

// Generate mock IDs using cryptographically secure randomness
function generateId() {
  // Generate 16 random bytes and convert to hex string (32 characters)
  return crypto.randomBytes(16).toString("hex");
}

// Generate CSRF token
function generateCSRF() {
  return "csrf-" + generateId();
}

// Enhanced logging function
function log(message, data = null) {
  const timestamp = new Date().toISOString();
  console.log(`[${timestamp}] Mock Kratos: ${message}`);
  if (config.verbose && data) {
    console.log("  Data:", JSON.stringify(data, null, 2));
  }
}

// Response helper with delay
async function sendResponse(res, statusCode, data, contentType = "application/json") {
  if (config.responseDelay > 0) {
    await new Promise((resolve) => setTimeout(resolve, config.responseDelay));
  }

  res.statusCode = statusCode;
  res.setHeader("Content-Type", contentType);
  res.end(typeof data === "string" ? data : JSON.stringify(data));
}

// Error response helper
async function sendError(res, statusCode, message, errorId = null) {
  const error = {
    error: {
      message,
      ...(errorId && { id: errorId }),
      code: statusCode,
      status: getStatusText(statusCode),
    },
  };

  log(`Error ${statusCode}: ${message}`, error);
  await sendResponse(res, statusCode, error);
}

// Get HTTP status text
function getStatusText(code) {
  const statusTexts = {
    400: "Bad Request",
    401: "Unauthorized",
    403: "Forbidden",
    404: "Not Found",
    410: "Gone",
    500: "Internal Server Error",
  };
  return statusTexts[code] || "Unknown";
}

// Create a mock login flow
function createLoginFlow(returnTo = "http://localhost:3010/") {
  const flowId = generateId();
  const csrf = generateCSRF();
  const flow = {
    id: flowId,
    expires_at: new Date(Date.now() + config.flowExpiration).toISOString(), // Configurable expiration
    issued_at: new Date().toISOString(),
    request_url: `http://localhost:4545/self-service/login/browser?return_to=${encodeURIComponent(returnTo)}`,
    return_to: returnTo,
    type: "browser",
    ui: {
      action: `/self-service/login?flow=${flowId}`,
      method: "POST",
      nodes: [
        {
          type: "input",
          group: "default",
          attributes: {
            name: "csrf_token",
            type: "hidden",
            value: csrf,
            required: true,
            disabled: false,
          },
          messages: [],
          meta: {},
        },
        {
          type: "input",
          group: "password",
          attributes: {
            name: "identifier",
            type: "email",
            required: true,
            disabled: false,
          },
          messages: [],
          meta: { label: { id: 1070004, text: "Email", type: "info" } },
        },
        {
          type: "input",
          group: "password",
          attributes: {
            name: "password",
            type: "password",
            required: true,
            disabled: false,
          },
          messages: [],
          meta: { label: { id: 1070001, text: "Password", type: "info" } },
        },
        {
          type: "input",
          group: "password",
          attributes: {
            name: "method",
            type: "submit",
            value: "password",
            disabled: false,
          },
          messages: [],
          meta: { label: { id: 1010001, text: "Sign in", type: "info" } },
        },
      ],
      messages: [],
    },
  };

  flows.set(flowId, { flow, csrf, expired: false });
  return flow;
}

// Handle POST data
function parsePostData(req, callback) {
  let body = "";
  req.on("data", (chunk) => {
    body += chunk.toString();
  });
  req.on("end", () => {
    try {
      const contentType = req.headers["content-type"] || "";
      if (contentType.includes("application/x-www-form-urlencoded")) {
        callback(null, querystring.parse(body));
      } else if (contentType.includes("application/json")) {
        callback(null, JSON.parse(body));
      } else {
        callback(null, querystring.parse(body)); // fallback
      }
    } catch (err) {
      callback(err, null);
    }
  });
}

const server = http.createServer(async (req, res) => {
  const parsedUrl = url.parse(req.url, true);
  const path = parsedUrl.pathname;
  const query = parsedUrl.query;

  log(`${req.method} ${req.url}`);

  // Enhanced CORS headers with more comprehensive support
  res.setHeader("Access-Control-Allow-Origin", "http://localhost:3010");
  res.setHeader("Access-Control-Allow-Credentials", "true");
  res.setHeader("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH");
  res.setHeader(
    "Access-Control-Allow-Headers",
    "Content-Type, Authorization, Cookie, X-Requested-With, Accept, Origin, User-Agent, DNT, Cache-Control, X-Mx-ReqToken, Keep-Alive, X-Requested-With, If-Modified-Since"
  );
  res.setHeader("Access-Control-Expose-Headers", "Set-Cookie");
  res.setHeader("Access-Control-Max-Age", "86400"); // 24 hours

  // Enhanced logging for debugging
  const cookies = req.headers.cookie || "";
  if (config.verbose) {
    log("Request headers:", req.headers);
    log("Request cookies:", cookies);
  }

  // Add preflight handling
  if (req.method === "OPTIONS") {
    await sendResponse(res, 200, "");
    return;
  }

  try {
    // Handle login browser flow creation
    if (req.method === "GET" && path === "/self-service/login/browser") {
      const returnTo = query.return_to || "http://localhost:3010/";
      const flow = createLoginFlow(returnTo);

      // Redirect to the login page with flow ID
      res.statusCode = 303;
      res.setHeader(
        "Location",
        `http://localhost:3010/auth/login?flow=${flow.id}&return_to=${encodeURIComponent(returnTo)}`
      );
      res.end();
      return;
    }

    // Handle flow retrieval
    if (req.method === "GET" && path === "/self-service/login/flows") {
      const flowId = query.id;
      if (!flowId) {
        await sendError(res, 400, "Flow ID is required");
        return;
      }

      const flowData = flows.get(flowId);
      if (!flowData) {
        await sendError(res, 404, "Flow not found");
        return;
      }

      // Check if flow is expired (for testing expired flow scenarios)
      if (
        flowId === "expired-flow-id" ||
        flowData.expired ||
        (flowData.flow && new Date(flowData.flow.expires_at) < new Date())
      ) {
        // Remove expired flow
        flows.delete(flowId);
        await sendError(
          res,
          410,
          "The login flow expired 1.234 minutes ago. Please try again.",
          "self_service_flow_expired"
        );
        return;
      }

      log(`Retrieved flow: ${flowId}`, flowData.flow);
      await sendResponse(res, 200, flowData.flow);
      return;
    }

    // Handle login form submission
    if (req.method === "POST" && path.startsWith("/self-service/login")) {
      const flowMatch = path.match(/flow=([^&]+)/) || (query.flow ? [null, query.flow] : null);
      if (!flowMatch) {
        res.statusCode = 400;
        res.setHeader("Content-Type", "application/json");
        res.end(JSON.stringify({ error: { message: "Flow ID is required" } }));
        return;
      }

      const flowId = flowMatch[1];
      const flowData = flows.get(flowId);

      if (!flowData) {
        res.statusCode = 404;
        res.setHeader("Content-Type", "application/json");
        res.end(JSON.stringify({ error: { message: "Flow not found" } }));
        return;
      }

      // Check for expired flow during submission
      if (flowId === "expired-flow-submission-id" || flowData.expired) {
        res.statusCode = 410;
        res.setHeader("Content-Type", "application/json");
        res.end(
          JSON.stringify({
            error: {
              id: "self_service_flow_expired",
              code: 410,
              status: "Gone",
              message: "The login flow expired during submission. Please try again.",
            },
          })
        );
        return;
      }

      parsePostData(req, (err, data) => {
        if (err) {
          res.statusCode = 400;
          res.setHeader("Content-Type", "application/json");
          res.end(JSON.stringify({ error: { message: "Invalid request data" } }));
          return;
        }

        const { identifier, password, csrf_token, method } = data;

        // CSRF validation
        if (csrf_token !== flowData.csrf) {
          res.statusCode = 403;
          res.setHeader("Content-Type", "application/json");
          res.end(
            JSON.stringify({
              error: {
                id: "security_csrf_violation",
                code: 403,
                status: "Forbidden",
                message: "A security violation was detected. Please retry the flow.",
              },
            })
          );
          return;
        }

        // Check credentials
        if (identifier === "wrong@example.com" || password === "wrongpassword") {
          // Return updated flow with error
          const updatedFlow = { ...flowData.flow };
          updatedFlow.ui.messages = [
            {
              id: 4000006,
              text: 'The provided credentials are invalid. Check for spelling mistakes in your email address, or <a href="/self-service/recovery/browser">recover your account</a>.',
              type: "error",
              context: {},
            },
          ];

          res.statusCode = 400;
          res.setHeader("Content-Type", "application/json");
          res.end(JSON.stringify(updatedFlow));
          return;
        }

        // Success case - create session
        const sessionId = generateId();
        const session = {
          id: sessionId,
          active: true,
          identity: {
            id: "mock-user-id",
            schema_id: "default",
            traits: {
              email: identifier,
              name: "Mock User",
            },
          },
        };

        sessions.set(sessionId, session);

        // Set session cookie with Domain=localhost to share across ports
        res.setHeader(
          "Set-Cookie",
          `ory_kratos_session=${sessionId}; Domain=localhost; HttpOnly; Path=/; SameSite=Lax`
        );
        res.statusCode = 200;
        res.setHeader("Content-Type", "application/json");

        // Return response in Kratos SuccessfulNativeLogin format
        res.end(
          JSON.stringify({
            session,
            // Kratos uses continue_with array with redirect_browser_to
            continue_with: [
              {
                action: "show_verification_ui",
                flow: {
                  id: flowId,
                  verifiable_address: identifier,
                },
              },
              {
                action: "redirect_browser_to",
                redirect_browser_to: flowData.flow.return_to || "http://localhost:3010/",
              },
            ],
          })
        );
      });
      return;
    }

    // Handle session validation (whoami)
    if (req.method === "GET" && path === "/sessions/whoami") {
      const cookies = req.headers.cookie || "";
      const sessionMatch = cookies.match(/ory_kratos_session=([^;]+)/);

      if (!sessionMatch) {
        res.statusCode = 401;
        res.setHeader("Content-Type", "application/json");
        res.end(JSON.stringify({ error: { message: "No active session found" } }));
        return;
      }

      const sessionId = sessionMatch[1];
      const session = sessions.get(sessionId);

      if (!session) {
        res.statusCode = 401;
        res.setHeader("Content-Type", "application/json");
        res.end(JSON.stringify({ error: { message: "Invalid session" } }));
        return;
      }

      res.statusCode = 200;
      res.setHeader("Content-Type", "application/json");
      res.end(JSON.stringify(session));
      return;
    }

    // Handle session validation (auth-hub compatible endpoint)
    if (req.method === "GET" && path === "/session") {
      const cookies = req.headers.cookie || "";
      const sessionMatch = cookies.match(/ory_kratos_session=([^;]+)/);

      log(`Session validation request, cookies: ${cookies}`);

      if (!sessionMatch) {
        log("No session cookie found");
        res.statusCode = 401;
        res.setHeader("Content-Type", "application/json");
        res.end(JSON.stringify({ error: { message: "No active session found" } }));
        return;
      }

      const sessionId = sessionMatch[1];
      const session = sessions.get(sessionId);

      if (!session) {
        log(`Invalid session ID: ${sessionId}`);
        res.statusCode = 401;
        res.setHeader("Content-Type", "application/json");
        res.end(JSON.stringify({ error: { message: "Invalid session" } }));
        return;
      }

      log(`Valid session found: ${sessionId}`);
      res.statusCode = 200;
      res.setHeader("Content-Type", "application/json");
      res.end(JSON.stringify(session));
      return;
    }

    // Mock backend API endpoints for feed stats
    if (req.method === "GET" && path === "/v1/feeds/stats") {
      const mockStats = {
        totalFeeds: 42,
        totalArticles: 1337,
        unreadCount: 15,
        dailyReadCount: 23,
      };
      log(`Mock API: Returning feed stats`, mockStats);
      await sendResponse(res, 200, mockStats);
      return;
    }

    // Mock backend API endpoint for feeds list
    if (req.method === "GET" && path === "/v1/feeds") {
      const mockFeeds = [
        {
          id: 1,
          title: "Mock Feed 1",
          url: "https://example1.com/feed",
          unreadCount: 5,
        },
        {
          id: 2,
          title: "Mock Feed 2",
          url: "https://example2.com/feed",
          unreadCount: 10,
        },
      ];
      log(`Mock API: Returning feeds list`, mockFeeds);
      await sendResponse(res, 200, { feeds: mockFeeds });
      return;
    }

    // Legacy auth validation endpoint
    if (req.method === "GET" && req.url === "/v1/auth/validate") {
      res.statusCode = 200;
      res.setHeader("Content-Type", "application/json");
      res.setHeader("Set-Cookie", "auth_session=mock; HttpOnly");
      res.end(JSON.stringify({ id: "mock-user", name: "Mock User" }));
      return;
    }

    // 404 for unhandled routes
    await sendError(res, 404, "Not found");
  } catch (error) {
    log("Unexpected error:", error);
    try {
      await sendError(res, 500, "Internal server error");
    } catch (sendError) {
      log("Failed to send error response:", sendError);
      res.statusCode = 500;
      res.end("Internal Server Error");
    }
  }
});

server.listen(port, () => {
  log(`Mock auth service running on port ${port}`);
  log(`Configuration:`, config);
});

// Session cleanup every 5 minutes
setInterval(
  () => {
    const now = Date.now();
    let cleanedSessions = 0;
    let cleanedFlows = 0;

    // Clean up expired sessions (older than 1 hour)
    for (const [sessionId, session] of sessions.entries()) {
      if (now - session.createdAt > 60 * 60 * 1000) {
        sessions.delete(sessionId);
        cleanedSessions++;
      }
    }

    // Clean up expired flows (older than 10 minutes)
    for (const [flowId, flowData] of flows.entries()) {
      const flowTime = new Date(flowData.flow.issued_at).getTime();
      if (now - flowTime > 10 * 60 * 1000) {
        flows.delete(flowId);
        cleanedFlows++;
      }
    }

    if (cleanedSessions > 0 || cleanedFlows > 0) {
      log(`Cleanup: Removed ${cleanedSessions} expired sessions and ${cleanedFlows} expired flows`);
    }
  },
  5 * 60 * 1000
);

// Gracefully shut down on termination signals
const close = () => {
  log("Shutting down mock auth service...");
  server.close(() => {
    log("Mock auth service stopped");
    process.exit(0);
  });
};
process.on("SIGINT", close);
process.on("SIGTERM", close);
