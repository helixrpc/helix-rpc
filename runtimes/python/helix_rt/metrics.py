"""
helix_rt.metrics
~~~~~~~~~~~~~~~~
Prometheus-compatible metrics endpoint for the Python Helix runtime.

Exposes a ``/metrics`` scrape endpoint in the Prometheus text exposition format.
Tracks request counts, error counts, and latency histograms per route.

Usage::

    from helix_rt.metrics import MetricsCollector

    collector = MetricsCollector()
    server.add_middleware(collector.middleware())
    server.register_route("GET", "/metrics", collector.handler)
    server.start()

Optional: install ``prometheus_client`` for richer output:
    pip install prometheus_client
"""

from __future__ import annotations

import time
from collections import defaultdict
from typing import Callable

from aiohttp import web


class MetricsCollector:
    """
    Lightweight, zero-dependency Prometheus metrics collector.

    Tracks per-route:
    - helix_requests_total{method, path, status}
    - helix_request_duration_seconds{method, path, le} (histogram)
    - helix_errors_total{method, path}
    """

    # Histogram bucket boundaries in seconds (matches Prometheus defaults)
    BUCKETS = [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0]

    def __init__(self) -> None:
        self._requests:  dict[tuple, int]         = defaultdict(int)
        self._errors:    dict[tuple, int]          = defaultdict(int)
        self._durations: dict[tuple, list[float]]  = defaultdict(list)

    def _record(self, method: str, path: str, status: int, duration: float) -> None:
        key = (method.upper(), path, status)
        self._requests[key] += 1
        self._durations[(method.upper(), path)] += [duration]
        if status >= 500:
            self._errors[(method.upper(), path)] += 1

    def middleware(self) -> Callable:
        """Return an aiohttp middleware that records per-request metrics."""
        collector = self

        @web.middleware
        async def _middleware(request: web.Request, handler: Callable) -> web.StreamResponse:
            start   = time.monotonic()
            status  = 500
            try:
                response = await handler(request)
                status  = response.status
                return response
            finally:
                duration = time.monotonic() - start
                collector._record(request.method, request.path, status, duration)

        return _middleware

    async def handler(self, _body: dict) -> web.Response:
        """Handler registered as GET /metrics — returns Prometheus text format."""
        lines = [
            "# HELP helix_requests_total Total number of RPC requests.",
            "# TYPE helix_requests_total counter",
        ]
        for (method, path, status), count in self._requests.items():
            lines.append(
                f'helix_requests_total{{method="{method}",path="{path}",status="{status}"}} {count}'
            )

        lines += [
            "",
            "# HELP helix_errors_total Total number of 5xx errors.",
            "# TYPE helix_errors_total counter",
        ]
        for (method, path), count in self._errors.items():
            lines.append(
                f'helix_errors_total{{method="{method}",path="{path}"}} {count}'
            )

        lines += [
            "",
            "# HELP helix_request_duration_seconds Request latency histogram.",
            "# TYPE helix_request_duration_seconds histogram",
        ]
        for (method, path), durations in self._durations.items():
            total   = sum(durations)
            count   = len(durations)
            for le in self.BUCKETS:
                bucket_count = sum(1 for d in durations if d <= le)
                lines.append(
                    f'helix_request_duration_seconds_bucket{{method="{method}",path="{path}",le="{le}"}} {bucket_count}'
                )
            lines.append(
                f'helix_request_duration_seconds_bucket{{method="{method}",path="{path}",le="+Inf"}} {count}'
            )
            lines.append(
                f'helix_request_duration_seconds_sum{{method="{method}",path="{path}"}} {total:.6f}'
            )
            lines.append(
                f'helix_request_duration_seconds_count{{method="{method}",path="{path}"}} {count}'
            )

        body = "\n".join(lines) + "\n"
        return web.Response(
            status=200,
            content_type="text/plain; version=0.0.4",
            body=body.encode(),
        )
