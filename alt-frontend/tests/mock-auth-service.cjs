const http = require("http");

const port = process.env.MOCK_AUTH_PORT || 4545;

const server = http.createServer((req, res) => {
  if (req.method === "GET" && req.url === "/v1/auth/validate") {
    res.statusCode = 200;
    res.setHeader("Content-Type", "application/json");
    // Mimic auth-service session cookie so downstream requests are authenticated
    res.setHeader("Set-Cookie", "auth_session=mock; HttpOnly");
    res.end(
      JSON.stringify({ id: "mock-user", name: "Mock User" })
    );
    return;
  }
  res.statusCode = 404;
  res.end();
});

server.listen(port, () => {
  console.log(`Mock auth service running on port ${port}`);
});

// Gracefully shut down on termination signals
const close = () => server.close(() => process.exit(0));
process.on("SIGINT", close);
process.on("SIGTERM", close);

