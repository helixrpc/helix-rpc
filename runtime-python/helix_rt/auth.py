"""
helix_rt.auth
~~~~~~~~~~~~~
JWT and API-key authentication middleware for the Python Helix runtime.

Usage (JWT)::

    from helix_rt.auth import jwt_middleware
    from helix_rt.server import HelixServer

    server = HelixServer()
    server.add_middleware(jwt_middleware(secret="my-secret", algorithms=["HS256"]))
    server.register_route("POST", "/predict", predict_handler)
    server.start()

Usage (API key)::

    from helix_rt.auth import api_key_middleware
    server.add_middleware(api_key_middleware(valid_keys={"sk-abc": "user1"}))
"""

from __future__ import annotations

import functools
import json
from typing import Callable, Awaitable, Optional

from aiohttp import web

from .errors import HelixError, ErrorCode


# ---------------------------------------------------------------------------
# JWT Middleware
# ---------------------------------------------------------------------------

def jwt_middleware(
    secret: str | bytes,
    algorithms: list[str] | None = None,
    required_claims: list[str] | None = None,
    token_header: str = "Authorization",
    audience: Optional[str] = None,
):
    """
    aiohttp middleware that validates a JWT Bearer token on every request.

    On success, the parsed claims dict is attached to ``request["jwt_claims"]``.
    On failure, returns a 401 HelixError response.

    Requires PyJWT: ``pip install PyJWT``
    """
    try:
        import jwt as pyjwt
    except ImportError as e:
        raise ImportError(
            "PyJWT is required for JWT auth: pip install PyJWT"
        ) from e

    if algorithms is None:
        algorithms = ["HS256"]
    if isinstance(secret, str):
        secret = secret.encode()

    @web.middleware
    async def middleware(request: web.Request, handler: Callable) -> web.StreamResponse:
        raw = _extract_bearer(request, token_header)
        if raw is None:
            return _auth_error("missing or malformed Authorization header")

        try:
            decode_kwargs: dict = {"algorithms": algorithms}
            if audience:
                decode_kwargs["audience"] = audience
            claims = pyjwt.decode(raw, secret, **decode_kwargs)
        except pyjwt.ExpiredSignatureError:
            return _auth_error("token has expired")
        except pyjwt.InvalidTokenError as e:
            return _auth_error(f"invalid token: {e}")

        # Verify required claims
        for claim in (required_claims or []):
            if claim not in claims:
                return _auth_error(f"missing required claim '{claim}'", status=403)

        request["jwt_claims"] = claims
        return await handler(request)

    return middleware


# ---------------------------------------------------------------------------
# API Key Middleware
# ---------------------------------------------------------------------------

def api_key_middleware(
    valid_keys: dict[str, str],
    header_name: str = "X-API-Key",
):
    """
    aiohttp middleware that validates a static API key.

    ``valid_keys`` maps API key → principal name.
    The resolved principal is stored in ``request["api_key_principal"]``.
    """

    @web.middleware
    async def middleware(request: web.Request, handler: Callable) -> web.StreamResponse:
        key = request.headers.get(header_name, "").strip()
        if not key:
            # Also accept Bearer style
            auth = request.headers.get("Authorization", "")
            if auth.startswith("Bearer "):
                key = auth[len("Bearer "):]

        if not key:
            return _auth_error(f"missing {header_name} header")

        principal = valid_keys.get(key)
        if principal is None:
            return _auth_error("invalid API key")

        request["api_key_principal"] = principal
        return await handler(request)

    return middleware


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _extract_bearer(request: web.Request, header: str) -> Optional[str]:
    value = request.headers.get(header, "").strip()
    if not value:
        return None
    if value.lower().startswith("bearer "):
        return value[7:].strip()
    return value  # also accept raw token


def _auth_error(message: str, status: int = 401) -> web.Response:
    code = ErrorCode.UNAUTHENTICATED if status == 401 else ErrorCode.PERMISSION_DENIED
    return web.Response(
        status=status,
        content_type="application/json",
        body=json.dumps({
            "error": message,
            "code": int(code),
        }).encode(),
    )
