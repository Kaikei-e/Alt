const http = require("node:http");
const _fs = require("node:fs");
const _path = require("node:path");

const port = 3010;

const server = http.createServer((req, res) => {
  console.log(`Request: ${req.method} ${req.url}`);

  // CORS headers
  res.setHeader("Access-Control-Allow-Origin", "*");
  res.setHeader(
    "Access-Control-Allow-Methods",
    "GET, POST, PUT, DELETE, OPTIONS",
  );
  res.setHeader("Access-Control-Allow-Headers", "Content-Type, Authorization");

  if (req.method === "OPTIONS") {
    res.statusCode = 200;
    res.end();
    return;
  }

  // Simple HTML response
  const html = `
<!DOCTYPE html>
<html>
<head>
    <title>Test Server</title>
</head>
<body>
    <h1>Test Server Running</h1>
    <p>This is a simple test server for debugging Playwright issues.</p>
</body>
</html>`;

  res.statusCode = 200;
  res.setHeader("Content-Type", "text/html");
  res.end(html);
});

server.listen(port, () => {
  console.log(`Test server running on port ${port}`);
});

// Graceful shutdown
process.on("SIGINT", () => {
  console.log("Shutting down test server...");
  server.close(() => {
    process.exit(0);
  });
});
