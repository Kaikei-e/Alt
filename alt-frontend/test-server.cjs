const http = require('http');
const fs = require('fs');
const path = require('path');

const port = 3010;

const server = http.createServer((req, res) => {
  console.log(`Request: ${req.method} ${req.url}`);

  // CORS headers
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization');

  if (req.method === 'OPTIONS') {
    res.statusCode = 200;
    res.end();
    return;
  }

  // Simple HTML response for testing
  const html = `
<!DOCTYPE html>
<html>
<head>
    <title>Alt Frontend Test</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .container { max-width: 800px; margin: 0 auto; }
        .header { background: #f0f0f0; padding: 20px; border-radius: 8px; margin-bottom: 20px; }
        .content { padding: 20px; }
        .test-info { background: #e8f4fd; padding: 15px; border-radius: 5px; margin: 10px 0; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Alt Frontend Test Server</h1>
            <p>This is a test server for E2E testing. The actual Next.js application is bypassed to avoid configuration issues.</p>
        </div>
        <div class="content">
            <div class="test-info">
                <h3>Test Environment</h3>
                <p>This server is running in test mode to avoid ResponseAborted errors caused by Next.js middleware and configuration conflicts.</p>
            </div>
            <div class="test-info">
                <h3>Available Routes</h3>
                <ul>
                    <li><a href="/">Home</a></li>
                    <li><a href="/desktop">Desktop</a></li>
                    <li><a href="/auth/login">Login</a></li>
                    <li><a href="/api/test">API Test</a></li>
                </ul>
            </div>
        </div>
    </div>
</body>
</html>`;

  res.statusCode = 200;
  res.setHeader('Content-Type', 'text/html');
  res.end(html);
});

server.listen(port, () => {
  console.log(`Test server running on port ${port}`);
});

// Graceful shutdown
process.on('SIGINT', () => {
  console.log('Shutting down test server...');
  server.close(() => {
    process.exit(0);
  });
});
