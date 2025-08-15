const http = require("http");

const port = process.env.MOCK_AUTH_PORT || 4545;

const server = http.createServer((req, res) => {
  if (req.method === "GET" && req.url === "/v1/auth/validate") {
    res.statusCode = 401;
    res.setHeader("Content-Type", "application/json");
    res.end("null");
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

