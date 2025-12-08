
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
        // Determine SSE URL. If localhost, use localhost:8502.
        // If remote, this simple logic might fail, but acceptable for now.
        const sseUrl = window.location.protocol + '//' + window.location.hostname + ':8502/stream';

        const evtSource = new EventSource(sseUrl);

        evtSource.onopen = function() {
            statusSpan.innerText = 'Connected';
            statusSpan.style.color = 'lime';
        };

        evtSource.onerror = function() {
            statusSpan.innerText = 'Disconnected';
            statusSpan.style.color = 'red';
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
                gpuContainer.innerHTML = '<div style="color: #aaa;">GPU Not Available</div>';
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
    </script>
    """

    # Height argument allows scrolling if content gets long
    components.html(html_code, height=800, scrolling=True)

