"""
helix_rt.errors
~~~~~~~~~~~~~~~
Structured error codes and exception types matching the Go and Rust runtimes.

Usage::

    from helix_rt.errors import HelixError, ErrorCode

    raise HelixError(ErrorCode.NOT_FOUND, "model weights not loaded")
"""

from __future__ import annotations

from enum import IntEnum
from typing import Optional


class ErrorCode(IntEnum):
    """gRPC-compatible status codes matching Go's ErrorCode and Rust's ErrorCode."""
    OK                = 0
    INVALID_ARGUMENT  = 3
    NOT_FOUND         = 5
    ALREADY_EXISTS    = 6
    PERMISSION_DENIED = 7
    UNIMPLEMENTED     = 12
    INTERNAL          = 13
    UNAVAILABLE       = 14
    UNAUTHENTICATED   = 16


# Map ErrorCode → HTTP status
_HTTP_STATUS: dict[ErrorCode, int] = {
    ErrorCode.OK:                200,
    ErrorCode.INVALID_ARGUMENT:  400,
    ErrorCode.NOT_FOUND:         404,
    ErrorCode.ALREADY_EXISTS:    409,
    ErrorCode.PERMISSION_DENIED: 403,
    ErrorCode.UNIMPLEMENTED:     501,
    ErrorCode.INTERNAL:          500,
    ErrorCode.UNAVAILABLE:       503,
    ErrorCode.UNAUTHENTICATED:   401,
}


class HelixError(Exception):
    """
    Structured RPC error that carries a gRPC status code and a human-readable message.

    The aiohttp handler in HelixServer will automatically convert this into the
    appropriate HTTP status code so callers receive a meaningful response.
    """

    def __init__(self, code: ErrorCode, message: str) -> None:
        super().__init__(message)
        self.code = code
        self.message = message

    def __str__(self) -> str:
        return f"helix error: code={self.code.name} message={self.message}"

    @property
    def http_status(self) -> int:
        return _HTTP_STATUS.get(self.code, 500)

    @property
    def grpc_status(self) -> str:
        return str(int(self.code))


def map_to_http_status(code: ErrorCode) -> int:
    """Return the HTTP status code for a given ErrorCode."""
    return _HTTP_STATUS.get(code, 500)
