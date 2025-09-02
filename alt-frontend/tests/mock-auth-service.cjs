const http = require("http");
const url = require("url");
const querystring = require("querystring");

const port = process.env.MOCK_AUTH_PORT || 4545;

// In-memory storage for flows
const flows = new Map();
const sessions = new Map();

// Generate mock IDs
function generateId() {
  return Math.random().toString(36).substring(2, 15) + Math.random().toString(36).substring(2, 15);
}

// Generate CSRF token
function generateCSRF() {
  return 'csrf-' + generateId();
}

// Create a mock login flow
function createLoginFlow(returnTo = 'http://localhost:3010/') {
  const flowId = generateId();
  const csrf = generateCSRF();
  const flow = {
    id: flowId,
    expires_at: new Date(Date.now() + 10 * 60 * 1000).toISOString(), // 10 minutes from now
    issued_at: new Date().toISOString(),
    request_url: `http://localhost:4545/self-service/login/browser?return_to=${encodeURIComponent(returnTo)}`,
    return_to: returnTo,
    type: 'browser',
    ui: {
      action: `/self-service/login?flow=${flowId}`,
      method: 'POST',
      nodes: [
        {
          type: 'input',
          group: 'default',
          attributes: {
            name: 'csrf_token',
            type: 'hidden',
            value: csrf,
            required: true,
            disabled: false
          },
          messages: [],
          meta: {}
        },
        {
          type: 'input',
          group: 'password',
          attributes: {
            name: 'identifier',
            type: 'email',
            required: true,
            disabled: false
          },
          messages: [],
          meta: { label: { id: 1070004, text: 'Email', type: 'info' } }
        },
        {
          type: 'input',
          group: 'password',
          attributes: {
            name: 'password',
            type: 'password',
            required: true,
            disabled: false
          },
          messages: [],
          meta: { label: { id: 1070001, text: 'Password', type: 'info' } }
        },
        {
          type: 'input',
          group: 'password',
          attributes: {
            name: 'method',
            type: 'submit',
            value: 'password',
            disabled: false
          },
          messages: [],
          meta: { label: { id: 1010001, text: 'Sign in', type: 'info' } }
        }
      ],
      messages: []
    }
  };
  
  flows.set(flowId, { flow, csrf, expired: false });
  return flow;
}

// Handle POST data
function parsePostData(req, callback) {
  let body = '';
  req.on('data', chunk => {
    body += chunk.toString();
  });
  req.on('end', () => {
    try {
      const contentType = req.headers['content-type'] || '';
      if (contentType.includes('application/x-www-form-urlencoded')) {
        callback(null, querystring.parse(body));
      } else if (contentType.includes('application/json')) {
        callback(null, JSON.parse(body));
      } else {
        callback(null, querystring.parse(body)); // fallback
      }
    } catch (err) {
      callback(err, null);
    }
  });
}

const server = http.createServer((req, res) => {
  const parsedUrl = url.parse(req.url, true);
  const path = parsedUrl.pathname;
  const query = parsedUrl.query;
  
  console.log(`Mock Kratos: ${req.method} ${req.url}`);
  
  // CORS headers
  res.setHeader('Access-Control-Allow-Origin', 'http://localhost:3010');
  res.setHeader('Access-Control-Allow-Credentials', 'true');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, X-Requested-With');
  
  if (req.method === 'OPTIONS') {
    res.statusCode = 200;
    res.end();
    return;
  }

  // Handle login browser flow creation
  if (req.method === 'GET' && path === '/self-service/login/browser') {
    const returnTo = query.return_to || 'http://localhost:3010/';
    const flow = createLoginFlow(returnTo);
    
    // Redirect to the login page with flow ID
    res.statusCode = 303;
    res.setHeader('Location', `http://localhost:3010/auth/login?flow=${flow.id}&return_to=${encodeURIComponent(returnTo)}`);
    res.end();
    return;
  }

  // Handle flow retrieval
  if (req.method === 'GET' && path === '/self-service/login/flows') {
    const flowId = query.id;
    if (!flowId) {
      res.statusCode = 400;
      res.setHeader('Content-Type', 'application/json');
      res.end(JSON.stringify({ error: { message: 'Flow ID is required' } }));
      return;
    }

    const flowData = flows.get(flowId);
    if (!flowData) {
      res.statusCode = 404;
      res.setHeader('Content-Type', 'application/json');
      res.end(JSON.stringify({ error: { message: 'Flow not found' } }));
      return;
    }

    // Check if flow is expired (for testing expired flow scenarios)
    if (flowId === 'expired-flow-id' || flowData.expired) {
      res.statusCode = 410;
      res.setHeader('Content-Type', 'application/json');
      res.end(JSON.stringify({
        error: {
          id: 'self_service_flow_expired',
          code: 410,
          status: 'Gone',
          message: 'The login flow expired 1.234 minutes ago. Please try again.'
        }
      }));
      return;
    }

    res.statusCode = 200;
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify(flowData.flow));
    return;
  }

  // Handle login form submission
  if (req.method === 'POST' && path.startsWith('/self-service/login')) {
    const flowMatch = path.match(/flow=([^&]+)/) || (query.flow ? [null, query.flow] : null);
    if (!flowMatch) {
      res.statusCode = 400;
      res.setHeader('Content-Type', 'application/json');
      res.end(JSON.stringify({ error: { message: 'Flow ID is required' } }));
      return;
    }

    const flowId = flowMatch[1];
    const flowData = flows.get(flowId);
    
    if (!flowData) {
      res.statusCode = 404;
      res.setHeader('Content-Type', 'application/json');
      res.end(JSON.stringify({ error: { message: 'Flow not found' } }));
      return;
    }

    // Check for expired flow during submission
    if (flowId === 'valid-flow-id' || flowData.expired) {
      res.statusCode = 410;
      res.setHeader('Content-Type', 'application/json');
      res.end(JSON.stringify({
        error: {
          id: 'self_service_flow_expired',
          code: 410,
          status: 'Gone'
        }
      }));
      return;
    }

    parsePostData(req, (err, data) => {
      if (err) {
        res.statusCode = 400;
        res.setHeader('Content-Type', 'application/json');
        res.end(JSON.stringify({ error: { message: 'Invalid request data' } }));
        return;
      }

      const { identifier, password, csrf_token, method } = data;

      // CSRF validation
      if (csrf_token !== flowData.csrf) {
        res.statusCode = 403;
        res.setHeader('Content-Type', 'application/json');
        res.end(JSON.stringify({
          error: {
            id: 'security_csrf_violation',
            code: 403,
            status: 'Forbidden',
            message: 'A security violation was detected. Please retry the flow.'
          }
        }));
        return;
      }

      // Check credentials
      if (identifier === 'wrong@example.com' || password === 'wrongpassword') {
        // Return updated flow with error
        const updatedFlow = { ...flowData.flow };
        updatedFlow.ui.messages = [{
          id: 4000006,
          text: 'The provided credentials are invalid. Check for spelling mistakes in your email address, or <a href="/self-service/recovery/browser">recover your account</a>.',
          type: 'error',
          context: {}
        }];
        
        res.statusCode = 400;
        res.setHeader('Content-Type', 'application/json');
        res.end(JSON.stringify(updatedFlow));
        return;
      }

      // Success case - create session
      const sessionId = generateId();
      const session = {
        id: sessionId,
        active: true,
        identity: {
          id: 'mock-user-id',
          schema_id: 'default',
          traits: {
            email: identifier,
            name: 'Mock User'
          }
        }
      };
      
      sessions.set(sessionId, session);
      
      // Set session cookie
      res.setHeader('Set-Cookie', `ory_kratos_session=${sessionId}; HttpOnly; Path=/; SameSite=Lax`);
      res.statusCode = 200;
      res.setHeader('Content-Type', 'application/json');
      res.end(JSON.stringify({
        session,
        redirect_to: flowData.flow.return_to || 'http://localhost:3010/'
      }));
    });
    return;
  }

  // Handle session validation (whoami)
  if (req.method === 'GET' && path === '/sessions/whoami') {
    const cookies = req.headers.cookie || '';
    const sessionMatch = cookies.match(/ory_kratos_session=([^;]+)/);
    
    if (!sessionMatch) {
      res.statusCode = 401;
      res.setHeader('Content-Type', 'application/json');
      res.end(JSON.stringify({ error: { message: 'No active session found' } }));
      return;
    }
    
    const sessionId = sessionMatch[1];
    const session = sessions.get(sessionId);
    
    if (!session) {
      res.statusCode = 401;
      res.setHeader('Content-Type', 'application/json');
      res.end(JSON.stringify({ error: { message: 'Invalid session' } }));
      return;
    }
    
    res.statusCode = 200;
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify(session));
    return;
  }

  // Legacy auth validation endpoint
  if (req.method === "GET" && req.url === "/v1/auth/validate") {
    res.statusCode = 200;
    res.setHeader("Content-Type", "application/json");
    res.setHeader("Set-Cookie", "auth_session=mock; HttpOnly");
    res.end(
      JSON.stringify({ id: "mock-user", name: "Mock User" })
    );
    return;
  }

  // 404 for unhandled routes
  res.statusCode = 404;
  res.setHeader('Content-Type', 'application/json');
  res.end(JSON.stringify({ error: { message: 'Not found' } }));
});

server.listen(port, () => {
  console.log(`Mock auth service running on port ${port}`);
});

// Gracefully shut down on termination signals
const close = () => server.close(() => process.exit(0));
process.on("SIGINT", close);
process.on("SIGTERM", close);

