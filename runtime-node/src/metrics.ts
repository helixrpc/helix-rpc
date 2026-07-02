export class MetricsRegistry {
    private requestCounts: Map<string, number> = new Map();
    private latencyTotals: Map<string, number> = new Map();
    private errorCounts: Map<string, number> = new Map();

    public recordRequest(method: string) {
        this.requestCounts.set(method, (this.requestCounts.get(method) || 0) + 1);
    }

    public recordLatency(method: string, durationMs: number) {
        this.latencyTotals.set(method, (this.latencyTotals.get(method) || 0) + durationMs);
    }

    public recordError(method: string) {
        this.errorCounts.set(method, (this.errorCounts.get(method) || 0) + 1);
    }

    public exportPrometheus(): string {
        let out = "";
        
        out += "# HELP helix_requests_total Total number of Helix requests\n";
        out += "# TYPE helix_requests_total counter\n";
        for (const [method, val] of this.requestCounts) {
            out += `helix_requests_total{method="${method}"} ${val}\n`;
        }

        out += "\n# HELP helix_request_latency_ms_total Cumulative request latency in milliseconds\n";
        out += "# TYPE helix_request_latency_ms_total counter\n";
        for (const [method, val] of this.latencyTotals) {
            out += `helix_request_latency_ms_total{method="${method}"} ${val.toFixed(2)}\n`;
        }

        out += "\n# HELP helix_errors_total Total number of failed Helix requests\n";
        out += "# TYPE helix_errors_total counter\n";
        for (const [method, val] of this.errorCounts) {
            out += `helix_errors_total{method="${method}"} ${val}\n`;
        }

        return out;
    }
}

export const globalRegistry = new MetricsRegistry();
