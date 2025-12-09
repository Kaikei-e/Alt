
import streamlit as st
import streamlit.components.v1 as components

def render_system_monitor():
    st.header("System Monitor (Real-time)")

    # Client-side code to connect to SSE stream
    # Note: connect to host:8502 (mapped to container:8000)
    # Using relative URL in JS might not work if streamlit is on 8501 and SSE on 8502.
    # We must assume the user visits localhost:8501, so localhost:8502 is the SSE endpoint.
    # If deployed, this needs env var adjustment, but for now strict 8502 port is plan.

    html_code = """
    <div id="monitor-container" style="font-family: sans-serif;">
        <div style="display: flex; gap: 20px; flex-wrap: wrap; margin-bottom: 20px;">
            <div style="flex: 1; min-width: 250px; background: #262730; padding: 15px; border-radius: 5px; color: white;">
                <h3>CPU Usage</h3>
                <div style="font-size: 2em;" id="cpu">Loading...</div>
                <div id="cpu-bar" style="height: 10px; background: #444; margin-top: 10px; border-radius: 5px; overflow: hidden;">
                    <div id="cpu-fill" style="height: 100%; width: 0%; background: #00bcd4; transition: width 0.5s;"></div>
                </div>
            </div>

            <div style="flex: 1; min-width: 250px; background: #262730; padding: 15px; border-radius: 5px; color: white;">
                <h3>Memory Usage</h3>
                <div style="font-size: 2em;" id="mem">Loading...</div>
                <div style="font-size: 0.8em; color: #aaa;" id="mem-detail"></div>
                <div id="mem-bar" style="height: 10px; background: #444; margin-top: 10px; border-radius: 5px; overflow: hidden;">
                    <div id="mem-fill" style="height: 100%; width: 0%; background: #4caf50; transition: width 0.5s;"></div>
                </div>
            </div>

            <div style="flex: 1; min-width: 250px; background: #262730; padding: 15px; border-radius: 5px; color: white;">
                <h3>Hanging Processes</h3>
                <div style="font-size: 2em;" id="hang">Loading...</div>
                <div style="font-size: 0.8em; color: #aaa;">spawn_main / fork</div>
            </div>
        </div>

        <div id="gpu-container" style="display: flex; gap: 20px; flex-wrap: wrap; margin-bottom: 20px;">
            <!-- GPU cards injected here -->
        </div>

        <div style="background: #262730; padding: 15px; border-radius: 5px; color: white;">
            <h3>Top Processes</h3>
            <table style="width: 100%; border-collapse: collapse; font-size: 0.9em;">
                <thead style="background: #333;">
                    <tr>
                        <th style="padding: 8px; text-align: left;">PID</th>
                        <th style="padding: 8px; text-align: left;">Name</th>
                        <th style="padding: 8px; text-align: left;">CPU %</th>
                        <th style="padding: 8px; text-align: left;">Mem (MB)</th>
                    </tr>
                </thead>
                <tbody id="proc-body">
                </tbody>
            </table>
        </div>

        <div style="margin-top: 10px; font-size: 0.8em; color: #666; text-align: right;">
            Status: <span id="conn-status" style="color: orange;">Connecting...</span>
        </div>
    </div>

    <script>
        const statusSpan = document.getElementById('conn-status');
        // Determine SSE URL. Handle cases where window.location might not be reliable
        // (e.g., in Streamlit iframes or special protocols)
        function getSSEUrl() {
            // Try to get hostname from window.location
            let hostname = window.location.hostname;
            let protocol = window.location.protocol;

            // Handle special cases
            if (!hostname || hostname === '' || hostname === 'null') {
                hostname = 'localhost';
            }

            // If protocol is invalid (about:, file:, etc.), use http:
            if (!protocol || protocol === 'about:' || protocol === 'file:' || !protocol.includes('http')) {
                protocol = 'http:';
            }

            // Construct URL
            return protocol + '//' + hostname + ':8502/stream';
        }

        const sseUrl = getSSEUrl();
        console.log('SSE URL constructed:', sseUrl);

        let evtSource = null;
        let reconnectAttempts = 0;
        const maxReconnectAttempts = 5;
        const reconnectDelay = 3000; // 3 seconds

        function connectSSE() {
            try {
                console.log('Attempting to connect to SSE:', sseUrl);
                statusSpan.innerText = 'Connecting...';
                statusSpan.style.color = 'orange';

                evtSource = new EventSource(sseUrl);

                evtSource.onopen = function() {
                    console.log('SSE connection opened');
                    statusSpan.innerText = 'Connected';
                    statusSpan.style.color = 'lime';
                    reconnectAttempts = 0; // Reset on successful connection
                };

                evtSource.onerror = function(event) {
                    console.error('SSE error:', event);
                    const readyState = evtSource.readyState;

                    if (readyState === EventSource.CONNECTING) {
                        statusSpan.innerText = 'Connecting...';
                        statusSpan.style.color = 'orange';
                    } else if (readyState === EventSource.CLOSED) {
                        statusSpan.innerText = 'Disconnected';
                        statusSpan.style.color = 'red';

                        // Attempt to reconnect
                        if (reconnectAttempts < maxReconnectAttempts) {
                            reconnectAttempts++;
                            console.log(`Reconnecting attempt ${reconnectAttempts}/${maxReconnectAttempts}...`);
                            setTimeout(function() {
                                if (evtSource) {
                                    evtSource.close();
                                }
                                connectSSE();
                            }, reconnectDelay);
                        } else {
                            statusSpan.innerText = 'Connection Failed (Max retries)';
                            statusSpan.style.color = 'red';
                            console.error('Max reconnection attempts reached');
                        }
                    }
                };

                evtSource.onmessage = function(event) {
                    const data = JSON.parse(event.data);

                    // CPU
                    document.getElementById('cpu').innerText = data.cpu.percent + '%';
                    document.getElementById('cpu-fill').style.width = data.cpu.percent + '%';

                    // Memory
                    const usedGb = (data.memory.used / 1073741824).toFixed(1);
                    const totalGb = (data.memory.total / 1073741824).toFixed(1);
                    document.getElementById('mem').innerText = data.memory.percent + '%';
                    document.getElementById('mem-detail').innerText = usedGb + ' / ' + totalGb + ' GB';
                    document.getElementById('mem-fill').style.width = data.memory.percent + '%';

                    // Hanging
                    document.getElementById('hang').innerText = data.hanging_count;

                    // GPU
                    const gpuContainer = document.getElementById('gpu-container');
                    if (data.gpu.available && data.gpu.gpus.length > 0) {
                        let html = '';
                        data.gpu.gpus.forEach(gpu => {
                            html += `
                            <div style="flex: 1; min-width: 250px; background: #262730; padding: 15px; border-radius: 5px; color: white;">
                                <h4>` + gpu.name + ` (` + gpu.index + `)</h4>
                                <div>Util: ` + gpu.utilization + `%</div>
                                <div style="height: 5px; background: #444; margin: 5px 0; border-radius: 3px;">
                                    <div style="height: 100%; width: ` + gpu.utilization + `%; background: #e91e63;"></div>
                                </div>
                                <div>Mem: ` + gpu.memory_percent + `%</div>
                                <div style="height: 5px; background: #444; margin: 5px 0; border-radius: 3px;">
                                    <div style="height: 100%; width: ` + gpu.memory_percent + `%; background: #9c27b0;"></div>
                                </div>
                                <div style="font-size: 0.8em; color: #aaa;">` + gpu.temperature + `°C</div>
                            </div>
                            `;
                        });
                        gpuContainer.innerHTML = html;
                    } else {
                        // Show more detailed error message if available
                        let message = 'GPU Not Available';
                        if (data.gpu.error) {
                            message += ' (' + data.gpu.error + ')';
                        } else if (data.gpu.message) {
                            message = data.gpu.message;
                        }
                        gpuContainer.innerHTML = '<div style="color: #aaa; padding: 10px;">' + message + '</div>';
                    }

                    // Processes
                    const tbody = document.getElementById('proc-body');
                    let rows = '';
                    data.top_processes.forEach(p => {
                        rows += `
                        <tr style="border-bottom: 1px solid #444;">
                            <td style="padding: 6px;">` + p.pid + `</td>
                            <td style="padding: 6px;">` + (p.name.length > 30 ? p.name.substring(0,30)+'...' : p.name) + `</td>
                            <td style="padding: 6px;">` + p.cpu_percent + `%</td>
                            <td style="padding: 6px;">` + p.memory_mb.toFixed(1) + `</td>
                        </tr>
                        `;
                    });
                    tbody.innerHTML = rows;
                };
            } catch (error) {
                console.error('Error creating SSE connection:', error);
                statusSpan.innerText = 'Connection Error: ' + error.message;
                statusSpan.style.color = 'red';
            }
        }

        // Initialize connection
        connectSSE();

        // Cleanup on page unload
        window.addEventListener('beforeunload', function() {
            if (evtSource) {
                evtSource.close();
            }
        });
    </script>
    """

    # Height argument allows scrolling if content gets long
    components.html(html_code, height=800, scrolling=True)

