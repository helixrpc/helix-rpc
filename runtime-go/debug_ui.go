package runtime

import (
	"fmt"
	"net/http"
)

// MountDebugUI mounts the Dev Dashboard at /__helix/ui.
func MountDebugUI(mux *http.ServeMux) {
	mux.HandleFunc("/__helix/ui", uiHandler)
	mux.HandleFunc("/__helix/ui/", uiHandler)
}

func uiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, dashboardHTML)
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Helix Dev Dashboard</title>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;600;800&family=JetBrains+Mono:wght@400;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-color: #0d0f14;
            --card-bg: #151922;
            --accent: #ff4a5a;
            --accent-green: #00e676;
            --accent-yellow: #ffd600;
            --text-main: #f5f6f9;
            --text-muted: #8b949e;
            --border-color: #21262d;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            background-color: var(--bg-color);
            color: var(--text-main);
            font-family: 'Outfit', sans-serif;
            padding: 2rem;
            min-height: 100vh;
        }

        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 2rem;
            border-bottom: 1px solid var(--border-color);
            padding-bottom: 1.5rem;
        }

        .logo {
            font-size: 2rem;
            font-weight: 800;
            background: linear-gradient(135deg, var(--accent), #ff8a80);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }

        .badge {
            font-family: 'JetBrains Mono', monospace;
            background-color: rgba(255, 74, 90, 0.15);
            color: var(--accent);
            border: 1px solid var(--accent);
            padding: 0.25rem 0.5rem;
            border-radius: 4px;
            font-size: 0.8rem;
        }

        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
            gap: 1.5rem;
            margin-bottom: 2rem;
        }

        .card {
            background-color: var(--card-bg);
            border: 1px solid var(--border-color);
            border-radius: 12px;
            padding: 1.5rem;
            position: relative;
            overflow: hidden;
            transition: transform 0.2s, border-color 0.2s;
        }

        .card:hover {
            transform: translateY(-2px);
            border-color: rgba(255, 74, 90, 0.4);
        }

        .card::before {
            content: '';
            position: absolute;
            top: 0;
            left: 0;
            width: 4px;
            height: 100%;
            background-color: var(--accent);
        }

        .card.green::before { background-color: var(--accent-green); }
        .card.yellow::before { background-color: var(--accent-yellow); }

        .card-title {
            font-size: 0.9rem;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            color: var(--text-muted);
            margin-bottom: 0.5rem;
        }

        .card-value {
            font-size: 2.2rem;
            font-weight: 800;
            font-family: 'JetBrains Mono', monospace;
        }

        .section-title {
            font-size: 1.3rem;
            font-weight: 600;
            margin-bottom: 1rem;
            color: var(--text-main);
        }

        table {
            width: 100%;
            border-collapse: collapse;
            margin-bottom: 2rem;
            background-color: var(--card-bg);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            overflow: hidden;
        }

        th, td {
            padding: 1rem;
            text-align: left;
            border-bottom: 1px solid var(--border-color);
        }

        th {
            background-color: rgba(255, 255, 255, 0.02);
            color: var(--text-muted);
            font-weight: 600;
            text-transform: uppercase;
            font-size: 0.8rem;
        }

        tr:last-child td {
            border-bottom: none;
        }

        .mono {
            font-family: 'JetBrains Mono', monospace;
        }

        .status-dot {
            display: inline-block;
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background-color: var(--accent-green);
            margin-right: 0.5rem;
        }

        .status-dot.open {
            background-color: var(--accent);
            box-shadow: 0 0 8px var(--accent);
        }
    </style>
</head>
<body>
    <header>
        <div class="logo">🧬 HELIX <span class="badge">DEV PANEL</span></div>
        <div class="mono" id="uptime">Uptime: 0s</div>
    </header>

    <div class="stats-grid">
        <div class="card" id="version-card">
            <div class="card-title">Version</div>
            <div class="card-value" id="version-val">0.2.0</div>
        </div>
        <div class="card green" id="circuit-card">
            <div class="card-title">Circuit Breaker</div>
            <div class="card-value" id="circuit-val">CLOSED</div>
        </div>
        <div class="card yellow" id="go-version-card">
            <div class="card-title">Go Version</div>
            <div class="card-value" style="font-size: 1.5rem; margin-top: 0.5rem;" id="go-val">unknown</div>
        </div>
    </div>

    <div class="section-title">Registered RPC Methods</div>
    <table>
        <thead>
            <tr>
                <th>RPC Method / Path</th>
                <th>Total Requests</th>
                <th>Errors</th>
                <th>Avg Latency</th>
            </tr>
        </thead>
        <tbody id="methods-body">
            <tr>
                <td colspan="4" style="text-align: center; color: var(--text-muted);">Waiting for metrics...</td>
            </tr>
        </tbody>
    </table>

    <div class="section-title">Backend Pool Status</div>
    <table>
        <thead>
            <tr>
                <th>Backend Target</th>
                <th>Active Connections</th>
            </tr>
        </thead>
        <tbody id="backends-body">
            <tr>
                <td colspan="2" style="text-align: center; color: var(--text-muted);">No backends monitored.</td>
            </tr>
        </tbody>
    </table>

    <script>
        async function updateDashboard() {
            try {
                const res = await fetch('/__helix/debug');
                if (!res.ok) return;
                const data = await res.json();

                document.getElementById('uptime').textContent = 'Uptime: ' + data.uptime;
                document.getElementById('version-val').textContent = data.helix_version || '0.2.0';
                document.getElementById('go-val').textContent = data.go_version || 'unknown';

                // Update Circuit State
                const circuitVal = document.getElementById('circuit-val');
                const circuitCard = document.getElementById('circuit-card');
                if (data.circuit_breaker) {
                    circuitVal.textContent = data.circuit_breaker.state;
                    if (data.circuit_breaker.state === 'OPEN') {
                        circuitCard.className = 'card';
                        circuitVal.style.color = 'var(--accent)';
                    } else if (data.circuit_breaker.state === 'HALF-OPEN') {
                        circuitCard.className = 'card yellow';
                        circuitVal.style.color = 'var(--accent-yellow)';
                    } else {
                        circuitCard.className = 'card green';
                        circuitVal.style.color = 'var(--accent-green)';
                    }
                } else {
                    circuitVal.textContent = 'N/A';
                }

                // Update Methods Table
                const methodsBody = document.getElementById('methods-body');
                if (data.methods && data.methods.length > 0) {
                    methodsBody.innerHTML = data.methods.map(m => 
                        '<tr>' +
                            '<td class="mono">' + m.method + '</td>' +
                            '<td>' + m.requests_total + '</td>' +
                            '<td>' + m.errors_total + '</td>' +
                            '<td class="mono">' + m.avg_latency_ms + 'ms</td>' +
                        '</tr>'
                    ).join('');
                } else {
                    methodsBody.innerHTML = '<tr><td colspan="4" style="text-align: center; color: var(--text-muted);">No requests recorded yet.</td></tr>';
                }

                // Update Backends Table
                const backendsBody = document.getElementById('backends-body');
                if (data.backends && data.backends.length > 0) {
                    backendsBody.innerHTML = data.backends.map(b => 
                        '<tr>' +
                            '<td class="mono">' + b.addr + '</td>' +
                            '<td>' + b.active_conns + '</td>' +
                        '</tr>'
                    ).join('');
                } else {
                    backendsBody.innerHTML = '<tr><td colspan="2" style="text-align: center; color: var(--text-muted);">No backends monitored.</td></tr>';
                }

            } catch (err) {
                console.error('Failed to fetch dashboard metrics:', err);
            }
        }

        setInterval(updateDashboard, 1000);
        updateDashboard();
    </script>
</body>
</html>
`
