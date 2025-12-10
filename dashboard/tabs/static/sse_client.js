// SSE Client Configuration (injected by Python)
// These values will be replaced by Python code
const SSE_HOST = '{{SSE_HOST}}';
const SSE_PORT = '{{SSE_PORT}}';
const SSE_PROTOCOL = '{{SSE_PROTOCOL}}';
const SSE_PATH = '{{SSE_PATH}}';
const TIMEOUT_MS = {{ TIMEOUT_MS }};
const MAX_RECONNECT_ATTEMPTS = {{ MAX_RECONNECT_ATTEMPTS }};
const RECONNECT_BASE_DELAY = 3000; // Base delay in milliseconds

// State management
let evtSource = null;
let reconnectAttempts = 0;
let connectionTimeout = null;
let reconnectTimeout = null;
let lastError = null;
let connectionStartTime = null;

const statusSpan = document.getElementById('conn-status');

// Diagnostic logging
function logDiagnostic(level, message, data = null) {
  const timestamp = new Date().toISOString();
  const logMessage = `[SSE ${timestamp}] ${message}`;
  if (data) {
    console[level](logMessage, data);
  } else {
    console[level](logMessage);
  }
}

function updateStatus(text, color, error = null) {
  if (statusSpan) {
    statusSpan.innerText = text;
    statusSpan.style.color = color;
  }
  if (error) {
    lastError = error;
    logDiagnostic('error', `Status update: ${text}`, error);
  } else {
    logDiagnostic('info', `Status update: ${text}`);
  }
}

// Determine SSE URL with fallback strategies
function getSSEUrl() {
  let hostname = SSE_HOST || 'localhost';
  let port = SSE_PORT || '8502';
  let protocol = SSE_PROTOCOL || 'http';

  // Method 1: Try parent window (for Streamlit iframes)
  // Note: In Streamlit components.html, we're in an iframe with about:srcdoc
  // We need to get the parent window's URL to determine the correct hostname
  let parentAccessAttempted = false;
  try {
    if (window.parent && window.parent !== window) {
      parentAccessAttempted = true;
      // Try to access parent location - this may throw in cross-origin scenarios
      const parentLocation = window.parent.location;
      const parentHost = parentLocation.hostname;
      const parentProtocol = parentLocation.protocol.replace(':', '');
      const parentPort = parentLocation.port;

      logDiagnostic('info', 'Attempting to access parent window', {
        parentHost,
        parentProtocol,
        parentPort,
        parentHref: parentLocation.href,
        currentLocation: window.location.href
      });

      // Only use parent if it's a valid hostname (not srcdoc, null, or empty)
      if (parentHost && parentHost !== '' && parentHost !== 'srcdoc' && parentHost !== 'null' && parentHost !== 'about') {
        hostname = parentHost;
        protocol = parentProtocol;
        // If parent is on localhost:8501, we should connect to localhost:8502 for SSE
        // So we keep the configured port (8502) instead of using parent's port
        logDiagnostic('info', 'Using parent window location', {
          hostname,
          protocol,
          port: port, // Keep configured port (8502)
          parentUrl: parentLocation.href,
          constructedUrl: `${protocol}://${hostname}:${port}/stream`
        });
      } else {
        logDiagnostic('info', 'Parent window has invalid hostname, using defaults', {
          parentHost,
          hostname: SSE_HOST || 'localhost',
          port: SSE_PORT || '8502'
        });
      }
    }
  } catch (e) {
    // Cross-origin or security error - this is expected in iframes
    // Use the configured defaults instead
    logDiagnostic('info', 'Cannot access parent window (cross-origin iframe), using configured defaults', {
      error: e.message,
      errorName: e.name,
      parentAccessAttempted: parentAccessAttempted,
      hostname: SSE_HOST || 'localhost',
      port: SSE_PORT || '8502'
    });
  }

  // Method 2: Try current window as fallback (usually won't work in srcdoc iframes)
  try {
    if (window.location && window.location.hostname &&
      window.location.hostname !== '' &&
      window.location.hostname !== 'srcdoc' &&
      window.location.hostname !== 'null' &&
      window.location.hostname !== 'about') {
      hostname = window.location.hostname;
      protocol = window.location.protocol.replace(':', '');
      logDiagnostic('info', 'Using current window location', { hostname, protocol });
    }
  } catch (e) {
    logDiagnostic('warn', 'Cannot access current window location', e.message);
  }

  // Final fallback: use values from Python configuration or defaults
  // This is the most reliable method for Streamlit components.html
  if (!hostname || hostname === '' || hostname === 'null' || hostname === 'srcdoc' || hostname === 'about') {
    hostname = SSE_HOST || 'localhost';
    logDiagnostic('info', 'Using configured/default hostname', { hostname, source: 'SSE_HOST or default' });
  }

  if (!protocol || protocol === 'about' || protocol === 'file' || !protocol.includes('http')) {
    protocol = SSE_PROTOCOL || 'http';
    logDiagnostic('info', 'Using configured/default protocol', { protocol, source: 'SSE_PROTOCOL or default' });
  }

  // Ensure port is always set (should be 8502 for host, which maps to 8000 in container)
  if (!port || port === '') {
    port = SSE_PORT || '8502';
  }

  // Get SSE path (default to /stream if not configured)
  const ssePath = SSE_PATH || '/stream';
  // Ensure path starts with /
  const normalizedPath = ssePath.startsWith('/') ? ssePath : '/' + ssePath;

  // Construct URL
  const url = `${protocol}://${hostname}:${port}${normalizedPath}`;
  logDiagnostic('info', 'Final SSE URL constructed', {
    url,
    hostname,
    port,
    protocol,
    sseHost: SSE_HOST,
    ssePort: SSE_PORT,
    sseProtocol: SSE_PROTOCOL,
    windowLocation: window.location.href,
    parentWindow: window.parent !== window ? 'different' : 'same'
  });
  return url;
}

// Clear timeouts
function clearTimeouts() {
  if (connectionTimeout) {
    clearTimeout(connectionTimeout);
    connectionTimeout = null;
  }
  if (reconnectTimeout) {
    clearTimeout(reconnectTimeout);
    reconnectTimeout = null;
  }
}

// Calculate exponential backoff delay
function getReconnectDelay(attempt) {
  const baseDelay = Math.min(
    RECONNECT_BASE_DELAY * Math.pow(2, attempt - 1),
    30000 // Max 30 seconds
  );
  const jitter = Math.floor(Math.random() * 1000); // Add randomness
  return baseDelay + jitter;
}

// Handle data updates
function handleDataUpdate(data) {
  try {
    // CPU
    const cpuElement = document.getElementById('cpu');
    const cpuFillElement = document.getElementById('cpu-fill');
    if (cpuElement && cpuFillElement) {
      cpuElement.innerText = data.cpu.percent + '%';
      cpuFillElement.style.width = data.cpu.percent + '%';
    }

    // Memory
    const memElement = document.getElementById('mem');
    const memDetailElement = document.getElementById('mem-detail');
    const memFillElement = document.getElementById('mem-fill');
    if (memElement && memDetailElement && memFillElement) {
      const usedGb = (data.memory.used / 1073741824).toFixed(1);
      const totalGb = (data.memory.total / 1073741824).toFixed(1);
      memElement.innerText = data.memory.percent + '%';
      memDetailElement.innerText = usedGb + ' / ' + totalGb + ' GB';
      memFillElement.style.width = data.memory.percent + '%';
    }

    // Hanging processes
    const hangElement = document.getElementById('hang');
    if (hangElement) {
      hangElement.innerText = data.hanging_count;
    }

    // GPU
    const gpuContainer = document.getElementById('gpu-container');
    if (gpuContainer) {
      if (data.gpu.available && data.gpu.gpus.length > 0) {
        let html = '';
        data.gpu.gpus.forEach(gpu => {
          html += `
                    <div style="flex: 1; min-width: 250px; background: #262730; padding: 15px; border-radius: 5px; color: white;">
                        <h4>${gpu.name} (${gpu.index})</h4>
                        <div>Util: ${gpu.utilization}%</div>
                        <div style="height: 5px; background: #444; margin: 5px 0; border-radius: 3px;">
                            <div style="height: 100%; width: ${gpu.utilization}%; background: #e91e63;"></div>
                        </div>
                        <div>Mem: ${gpu.memory_percent}%</div>
                        <div style="height: 5px; background: #444; margin: 5px 0; border-radius: 3px;">
                            <div style="height: 100%; width: ${gpu.memory_percent}%; background: #9c27b0;"></div>
                        </div>
                        <div style="font-size: 0.8em; color: #aaa;">${gpu.temperature}°C</div>
                    </div>
                    `;
        });
        gpuContainer.innerHTML = html;
      } else {
        let message = 'GPU Not Available';
        if (data.gpu.error) {
          message += ' (' + data.gpu.error + ')';
        } else if (data.gpu.message) {
          message = data.gpu.message;
        }
        gpuContainer.innerHTML = '<div style="color: #aaa; padding: 10px;">' + message + '</div>';
      }
    }

    // Processes - Top 10 only
    const tbody = document.getElementById('proc-body');
    if (tbody) {
      let rows = '';
      const top10Processes = data.top_processes.slice(0, 10);
      logDiagnostic('debug', `Updating ${top10Processes.length} processes`);
      top10Processes.forEach(p => {
        const truncatedName = p.name.length > 30 ? p.name.substring(0, 30) + '...' : p.name;
        rows += `
                <tr style="border-bottom: 1px solid #444;">
                    <td style="padding: 6px;">${p.pid}</td>
                    <td style="padding: 6px;">${truncatedName}</td>
                    <td style="padding: 6px;">${p.cpu_percent}%</td>
                    <td style="padding: 6px;">${p.memory_mb.toFixed(1)}</td>
                </tr>
                `;
      });
      tbody.innerHTML = rows;
    }
  } catch (error) {
    logDiagnostic('error', 'Error handling data update', error);
  }
}

// Test server availability before connecting
async function testServerAvailability(url) {
  const timeoutMs = 3000; // Reduced timeout for faster failure detection
  const abortController = new AbortController();
  const timeoutId = setTimeout(() => {
    abortController.abort();
  }, timeoutMs);

  try {
    // Extract base URL for health check
    // Replace /stream with /health in the path
    const urlObj = new URL(url);
    const pathname = urlObj.pathname;
    // Handle both /stream and /sse/dashboard/stream paths
    let healthPath = '/health';
    if (pathname.includes('/sse/dashboard/stream')) {
      // For /sse/dashboard/stream, use /sse/dashboard/health
      healthPath = pathname.replace(/\/stream$/, '/health');
    } else if (pathname.endsWith('/stream')) {
      // For /stream, use /health
      healthPath = pathname.replace(/\/stream$/, '/health');
    }
    const healthUrl = `${urlObj.protocol}//${urlObj.host}${healthPath}`;

    logDiagnostic('info', 'Testing server availability', {
      healthUrl,
      originalUrl: url,
      hostname: urlObj.hostname,
      port: urlObj.port,
      protocol: urlObj.protocol
    });

    const response = await fetch(healthUrl, {
      method: 'GET',
      mode: 'cors',
      cache: 'no-cache',
      credentials: 'omit', // Don't send cookies for health check
      signal: abortController.signal
    });

    clearTimeout(timeoutId);

    if (response.ok) {
      const data = await response.json();
      logDiagnostic('info', 'Server health check passed', data);
      return { available: true, health: data };
    } else {
      // Handle specific HTTP error codes
      let errorType = 'server_error';
      let errorMessage = `HTTP ${response.status}: ${response.statusText}`;

      if (response.status === 502) {
        errorType = 'bad_gateway';
        errorMessage = '502 Bad Gateway: Nginx cannot connect to dashboard service. Check if dashboard service is running.';
      } else if (response.status === 503) {
        errorType = 'service_unavailable';
        errorMessage = '503 Service Unavailable: Dashboard service is not available.';
      } else if (response.status === 504) {
        errorType = 'gateway_timeout';
        errorMessage = '504 Gateway Timeout: Dashboard service did not respond in time.';
      }

      logDiagnostic('warn', 'Server health check failed', {
        status: response.status,
        statusText: response.statusText,
        errorType: errorType,
        errorMessage: errorMessage
      });
      return { available: false, error: errorMessage, type: errorType, status: response.status };
    }
  } catch (error) {
    clearTimeout(timeoutId);

    // Determine error type with more detail
    let errorType = 'unknown_error';
    let errorMessage = error.message || 'Unknown error';

    if (error.name === 'AbortError' || error.name === 'TimeoutError') {
      errorType = 'timeout_error';
      errorMessage = `Health check timeout after ${timeoutMs}ms`;
    } else if (error.name === 'TypeError' && error.message.includes('Failed to fetch')) {
      errorType = 'network_error';
      // Extract port from URL for better error message
      let portInfo = '';
      let hostInfo = '';
      try {
        const urlObj = new URL(url);
        portInfo = urlObj.port ? `:${urlObj.port}` : '';
        hostInfo = urlObj.host;
      } catch (e) {
        // Ignore URL parsing errors
      }
      errorMessage = `Network error: Cannot reach SSE server at ${hostInfo}${portInfo}. Check if dashboard service is running and accessible via Nginx proxy.`;
    } else if (error.name === 'SecurityError') {
      errorType = 'security_error';
      errorMessage = 'Security error: CORS or same-origin policy violation. Check server CORS settings.';
    } else if (error.name === 'SyntaxError') {
      errorType = 'syntax_error';
      errorMessage = 'Syntax error: Invalid URL or configuration.';
    }

    logDiagnostic('error', 'Server availability test failed', {
      error: errorMessage,
      type: errorType,
      errorName: error.name,
      originalError: error.message
    });

    return {
      available: false,
      error: errorMessage,
      type: errorType
    };
  }
}

// Main connection function
async function connectSSE() {
  // Clean up any existing connection
  if (evtSource) {
    try {
      evtSource.close();
    } catch (e) {
      logDiagnostic('warn', 'Error closing existing connection', e);
    }
    evtSource = null;
  }

  clearTimeouts();

  const sseUrl = getSSEUrl();
  connectionStartTime = Date.now();
  reconnectAttempts++;

  logDiagnostic('info', `Connecting to SSE (attempt ${reconnectAttempts}/${MAX_RECONNECT_ATTEMPTS})`, {
    url: sseUrl,
    attempt: reconnectAttempts,
    maxAttempts: MAX_RECONNECT_ATTEMPTS
  });

  updateStatus('Connecting...', 'orange');

  // Test server availability first (with timeout to avoid blocking)
  // If health check fails, we'll still try to connect directly
  let serverTest = null;
  try {
    serverTest = await Promise.race([
      testServerAvailability(sseUrl),
      new Promise((resolve) => setTimeout(() => resolve({ available: false, error: 'Health check timeout', type: 'timeout_error' }), 3000))
    ]);
  } catch (e) {
    logDiagnostic('warn', 'Health check failed, will attempt direct SSE connection', { error: e.message });
    serverTest = { available: false, error: e.message, type: 'health_check_error' };
  }

  // If health check fails, log warning but still attempt SSE connection
  // SSE connection itself is the best test of server availability
  if (!serverTest || !serverTest.available) {
    logDiagnostic('warn', 'Health check failed, attempting direct SSE connection anyway', {
      error: serverTest?.error,
      type: serverTest?.type,
      url: sseUrl
    });
    // Continue to SSE connection attempt - don't return early
  }

  try {
    evtSource = new EventSource(sseUrl);

    // Set connection timeout
    connectionTimeout = setTimeout(() => {
      if (evtSource && evtSource.readyState !== EventSource.OPEN) {
        const elapsed = Date.now() - connectionStartTime;
        const readyStateName = evtSource.readyState === EventSource.CONNECTING ? 'CONNECTING' :
          evtSource.readyState === EventSource.OPEN ? 'OPEN' :
            evtSource.readyState === EventSource.CLOSED ? 'CLOSED' : 'UNKNOWN';

        logDiagnostic('error', 'Connection timeout', {
          url: sseUrl,
          readyState: evtSource.readyState,
          readyStateName: readyStateName,
          elapsed: elapsed,
          timeoutMs: TIMEOUT_MS,
          attempt: reconnectAttempts,
          maxAttempts: MAX_RECONNECT_ATTEMPTS
        });

        // Check if this might be a 502 Bad Gateway (connection closes immediately)
        let errorMessage = `Connection timeout after ${TIMEOUT_MS}ms (readyState: ${readyStateName})`;
        if (readyStateName === 'CLOSED' && elapsed < 2000) {
          // Very quick closure might indicate a 502 Bad Gateway
          errorMessage = `Connection failed immediately. Possible 502 Bad Gateway - check if dashboard service is running (requires 'recap' profile).`;
        }

        const error = {
          type: 'timeout',
          message: errorMessage,
          url: sseUrl,
          readyState: evtSource.readyState
        };

        updateStatus('Connection Timeout', 'red', error);

        if (evtSource) {
          evtSource.close();
          evtSource = null;
        }

        // Attempt to reconnect
        if (reconnectAttempts < MAX_RECONNECT_ATTEMPTS) {
          const delay = getReconnectDelay(reconnectAttempts);
          logDiagnostic('info', `Scheduling reconnect after timeout in ${delay}ms (attempt ${reconnectAttempts + 1}/${MAX_RECONNECT_ATTEMPTS})`, {
            delay: delay,
            elapsed: elapsed
          });
          reconnectTimeout = setTimeout(() => {
            connectSSE();
          }, delay);
        } else {
          updateStatus('Connection Failed (Max retries): Timeout', 'red', error);
          logDiagnostic('error', 'Max reconnection attempts reached after timeout', {
            finalElapsed: elapsed
          });
        }
      }
    }, TIMEOUT_MS);

    // Connection opened
    evtSource.onopen = function () {
      clearTimeouts();
      const elapsed = Date.now() - connectionStartTime;
      logDiagnostic('info', 'SSE connection opened successfully', {
        url: sseUrl,
        elapsed: elapsed,
        readyState: evtSource.readyState,
        attempt: reconnectAttempts,
        connectionTime: elapsed + 'ms'
      });

      updateStatus('Connected', 'lime');
      reconnectAttempts = 0; // Reset on successful connection
      lastError = null;
    };

    // Connection error
    evtSource.onerror = function (event) {
      const readyState = evtSource ? evtSource.readyState : EventSource.CLOSED;
      const elapsed = evtSource ? Date.now() - connectionStartTime : 0;

      // Determine error type
      let errorType = 'unknown';
      let errorMessage = 'Unknown error';

      if (readyState === EventSource.CONNECTING) {
        errorType = 'connecting';
        errorMessage = 'Connection in progress...';
        updateStatus('Connecting...', 'orange');
        logDiagnostic('info', 'Connection in progress', {
          readyState: readyState,
          elapsed: elapsed
        });
        return; // Don't treat CONNECTING as an error
      } else if (readyState === EventSource.CLOSED) {
        errorType = 'closed';
        // Check if this might be a 502 error (Bad Gateway)
        // EventSource doesn't expose HTTP status codes, but we can infer from the error
        if (elapsed < 1000 && lastError === null) {
          // Very quick closure might indicate a 502 Bad Gateway
          errorMessage = 'Connection closed immediately. Possible 502 Bad Gateway - check if dashboard service is running.';
          errorType = 'bad_gateway';
        } else {
          errorMessage = 'Connection closed';
        }

        logDiagnostic('error', 'SSE connection closed', {
          readyState: readyState,
          elapsed: elapsed,
          lastError: lastError
        });

        updateStatus('Disconnected', 'red', {
          type: errorType,
          message: errorMessage,
          readyState: readyState
        });

        clearTimeouts();

        // Attempt to reconnect
        if (reconnectAttempts < MAX_RECONNECT_ATTEMPTS) {
          const delay = getReconnectDelay(reconnectAttempts);
          logDiagnostic('info', `Scheduling reconnect after connection closed in ${delay}ms (attempt ${reconnectAttempts + 1}/${MAX_RECONNECT_ATTEMPTS})`, {
            delay: delay,
            elapsed: elapsed,
            readyState: readyState
          });
          reconnectTimeout = setTimeout(() => {
            connectSSE();
          }, delay);
        } else {
          updateStatus('Connection Failed (Max retries): Connection Closed', 'red', {
            type: errorType,
            message: errorMessage,
            readyState: readyState
          });
          logDiagnostic('error', 'Max reconnection attempts reached after connection closed', {
            finalElapsed: elapsed,
            readyState: readyState
          });
        }
      } else {
        errorType = 'error';
        errorMessage = 'Connection error';

        // Provide more specific error messages based on readyState
        if (readyState === EventSource.CONNECTING) {
          errorMessage = 'Connection attempt in progress (this is normal during initial connection)';
          errorType = 'connecting';
        } else if (readyState === EventSource.OPEN) {
          errorMessage = 'Connection error occurred but connection is still open';
          errorType = 'stream_error';
        } else {
          errorMessage = `Connection error (readyState: ${readyState})`;
          errorType = 'connection_error';
        }

        logDiagnostic('error', 'SSE error event', {
          readyState: readyState,
          readyStateName: readyState === EventSource.CONNECTING ? 'CONNECTING' :
            readyState === EventSource.OPEN ? 'OPEN' :
              readyState === EventSource.CLOSED ? 'CLOSED' : 'UNKNOWN',
          elapsed: elapsed,
          event: event,
          url: sseUrl
        });
      }
    };

    // Message received
    evtSource.onmessage = function (event) {
      clearTimeouts(); // Clear timeout on successful message

      try {
        const data = JSON.parse(event.data);
        logDiagnostic('debug', 'Received SSE message', {
          dataKeys: Object.keys(data),
          timestamp: new Date().toISOString()
        });

        handleDataUpdate(data);
      } catch (error) {
        logDiagnostic('error', 'Error parsing SSE message', {
          error: error.message,
          data: event.data
        });
      }
    };

  } catch (error) {
    clearTimeouts();

    // Determine error type and provide helpful message
    let errorType = 'exception';
    let errorMessage = error.message || 'Unknown error';
    let helpfulMessage = errorMessage;

    if (error.name === 'TypeError' && errorMessage.includes('Failed to fetch')) {
      errorType = 'network_error';
      helpfulMessage = 'Network error: Cannot reach SSE server. Check if server is running and port is accessible.';
    } else if (error.name === 'SecurityError') {
      errorType = 'security_error';
      helpfulMessage = 'Security error: CORS or same-origin policy violation. Check server CORS settings.';
    } else if (error.name === 'SyntaxError') {
      errorType = 'syntax_error';
      helpfulMessage = 'Syntax error: Invalid URL or configuration.';
    }

    logDiagnostic('error', 'Error creating SSE connection', {
      error: errorMessage,
      errorName: error.name,
      errorType: errorType,
      stack: error.stack,
      url: sseUrl,
      helpfulMessage: helpfulMessage
    });

    updateStatus(`Connection Error: ${helpfulMessage}`, 'red', {
      type: errorType,
      message: helpfulMessage,
      originalError: errorMessage,
      url: sseUrl
    });

    // Attempt to reconnect
    if (reconnectAttempts < MAX_RECONNECT_ATTEMPTS) {
      const delay = getReconnectDelay(reconnectAttempts);
      logDiagnostic('info', `Scheduling reconnect after exception in ${delay}ms (attempt ${reconnectAttempts + 1}/${MAX_RECONNECT_ATTEMPTS})`);
      reconnectTimeout = setTimeout(() => {
        connectSSE();
      }, delay);
    } else {
      updateStatus(`Connection Failed (Max retries): ${helpfulMessage}`, 'red', {
        type: errorType,
        message: helpfulMessage,
        originalError: errorMessage
      });
      logDiagnostic('error', 'Max reconnection attempts reached after exception', {
        errorType: errorType,
        helpfulMessage: helpfulMessage
      });
    }
  }
}

// Initialize connection
connectSSE();

// Cleanup on page unload
window.addEventListener('beforeunload', function () {
  logDiagnostic('info', 'Page unloading, cleaning up SSE connection');
  clearTimeouts();
  if (evtSource) {
    evtSource.close();
    evtSource = null;
  }
});

