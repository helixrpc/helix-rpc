"""
helix_rt.telemetry
~~~~~~~~~~~~~~~~~~
OpenTelemetry middleware for the Helix Python runtime.

Features:
  - W3C TraceContext (traceparent / tracestate) extraction
  - Server span creation with http.method / http.url / http.status_code attributes
  - Configurable probabilistic sampling so high-frequency prod traffic doesn't
    overwhelm the collector (default: 1 % of requests are traced).

Usage::

    from helix_rt.telemetry import telemetry_middleware          # default 1 %
    from helix_rt.telemetry import make_telemetry_middleware      # configurable
    from helix_rt.server import HelixServer

    # Trace 5 % of traffic:
    server = HelixServer()
    # The default middleware is already registered — to override, recreate
    # HelixServer with a custom app and register make_telemetry_middleware(0.05).
"""

from __future__ import annotations

import random
from enum import Enum
from functools import partial
from typing import Callable, Awaitable, Optional

from aiohttp import web
from opentelemetry import trace
from opentelemetry.propagate import extract

# The module-level tracer is cheap to create; the SDK only activates it when a
# global TracerProvider is configured.
_tracer = trace.get_tracer(__name__)


class SamplingStrategy(Enum):
    """Controls the fraction of requests that produce a trace span."""
    ALL = "all"             # 100 % — development/debug only
    NONE = "none"           # 0 %  — effectively disables telemetry
    PROBABILISTIC = "prob"  # configurable rate (default 1 %)


def _should_sample(strategy: SamplingStrategy, rate: float) -> bool:
    if strategy is SamplingStrategy.ALL:
        return True
    if strategy is SamplingStrategy.NONE:
        return False
    # PROBABILISTIC
    return random.random() < rate   # thread-safe: CPython GIL; fast for floats


def make_telemetry_middleware(
    sample_rate: float = 0.01,
    strategy: SamplingStrategy = SamplingStrategy.PROBABILISTIC,
) -> web.middleware:
    """
    Factory that returns an aiohttp middleware with the given sampling config.

    Args:
        sample_rate: Fraction [0.0, 1.0] of requests to trace.
                     Ignored when strategy != PROBABILISTIC.
        strategy:    Sampling strategy.  Defaults to 1 % probabilistic.
    """

    @web.middleware
    async def _telemetry_middleware(
        request: web.Request,
        handler: Callable[[web.Request], Awaitable[web.StreamResponse]],
    ) -> web.StreamResponse:
        if not _should_sample(strategy, sample_rate):
            return await handler(request)

        # Extract W3C TraceContext / B3 from incoming headers.
        context = extract(request.headers)

        with _tracer.start_as_current_span(
            f"{request.method} {request.path}",
            context=context,
            kind=trace.SpanKind.SERVER,
        ) as span:
            span.set_attribute("http.method", request.method)
            span.set_attribute("http.url", str(request.url))
            span.set_attribute("http.target", request.path)

            try:
                response = await handler(request)
                status = getattr(response, "status", 200)
                span.set_attribute("http.status_code", status)
                if status >= 500:
                    span.set_status(
                        trace.StatusCode.ERROR,
                        f"HTTP {status}",
                    )
                else:
                    span.set_status(trace.StatusCode.OK)
                return response
            except Exception as exc:
                span.record_exception(exc)
                span.set_status(trace.StatusCode.ERROR, str(exc))
                raise

    return _telemetry_middleware


# Convenience: pre-built default at 1 % sampling rate.
telemetry_middleware: web.middleware = make_telemetry_middleware(
    sample_rate=0.01,
    strategy=SamplingStrategy.PROBABILISTIC,
)
