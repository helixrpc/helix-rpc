"""
Comprehensive optimization tests for Helix RPC.

Covers:
  - LazyUserProfile zero-copy lazy field access
  - Protobuf → Thrift Compact transpilation
  - UserProfile.from_proto classmethod (zero-copy protobuf decode)
  - eBPF sockmap / UDS routing helper functions
"""

import sys
import os
# Mock pytest.raises so this file can run with pure python3/unittest
class raises:
    def __init__(self, expected_exception):
        self.expected_exception = expected_exception
    def __enter__(self):
        return self
    def __exit__(self, exc_type, exc_val, exc_tb):
        if exc_type is None:
            raise AssertionError(f"{self.expected_exception.__name__} not raised")
        return issubclass(exc_type, self.expected_exception)

class PytestMock:
    raises = raises

pytest = PytestMock()

# ---------------------------------------------------------------------------
# Path setup: generated models live next to this file (not installed).
# ---------------------------------------------------------------------------
_TESTS_PYTHON_DIR = os.path.dirname(os.path.abspath(__file__))
_RUNTIMES_PYTHON_DIR = os.path.join(
    _TESTS_PYTHON_DIR, "..", "..", "runtimes", "python"
)

if _TESTS_PYTHON_DIR not in sys.path:
    sys.path.insert(0, _TESTS_PYTHON_DIR)
if _RUNTIMES_PYTHON_DIR not in sys.path:
    sys.path.insert(0, _RUNTIMES_PYTHON_DIR)

from generated_models import (  # noqa: E402
    UserProfile,
    LazyUserProfile,
    _read_varint,
    _write_thrift_varint,
    _fast_scan_field,
)
from helix_rt.ebpf import (  # noqa: E402
    load_bpf_sockmap,
    has_unix_prefix,
    strip_unix_prefix,
)

# ---------------------------------------------------------------------------
# Shared protobuf fixture
# ---------------------------------------------------------------------------

def _build_proto(user_id: int, username: str, email: str) -> bytes:
    """Manually encode a UserProfile protobuf message (wire format)."""
    username_b = username.encode("utf-8")
    email_b = email.encode("utf-8")
    buf = bytearray()
    # field 1: user_id (varint, wire-type 0, tag = 0x08)
    buf.append(0x08)
    # encode user_id as varint
    v = user_id
    while True:
        if (v & ~0x7F) == 0:
            buf.append(v)
            break
        buf.append((v & 0x7F) | 0x80)
        v >>= 7
    # field 2: username (length-delimited, wire-type 2, tag = 0x12)
    buf.append(0x12)
    buf.append(len(username_b))
    buf.extend(username_b)
    # field 3: email (length-delimited, wire-type 2, tag = 0x1A)
    buf.append(0x1A)
    buf.append(len(email_b))
    buf.extend(email_b)
    return bytes(buf)


# Primary test payload matching the task specification
USERNAME = "zero_copy_hero"   # 14 bytes
EMAIL = "hero@helix.rpc"      # 14 bytes
USER_ID = 42

PROTO_BYTES = _build_proto(USER_ID, USERNAME, EMAIL)


# ===========================================================================
# 1–4: LazyUserProfile lazy field access
# ===========================================================================

class TestLazyUserProfileGetters:
    """Tests for zero-copy lazy field access on LazyUserProfile."""

    def test_lazy_user_profile_get_user_id(self):
        """LazyUserProfile.get_user_id() returns the correct integer."""
        profile = LazyUserProfile(PROTO_BYTES)
        assert profile.get_user_id() == USER_ID

    def test_lazy_user_profile_get_username(self):
        """LazyUserProfile.get_username() returns the correct string."""
        profile = LazyUserProfile(PROTO_BYTES)
        assert profile.get_username() == USERNAME

    def test_lazy_user_profile_get_email(self):
        """LazyUserProfile.get_email() returns the correct string."""
        profile = LazyUserProfile(PROTO_BYTES)
        assert profile.get_email() == EMAIL

    def test_lazy_independent_field_access(self):
        """Calling only get_email() (skipping user_id/username) returns correct value."""
        profile = LazyUserProfile(PROTO_BYTES)
        # Deliberately avoid calling get_user_id() and get_username() first
        email = profile.get_email()
        assert email == EMAIL

    def test_lazy_user_profile_reuse(self):
        """LazyUserProfile can be read multiple times without corruption."""
        profile = LazyUserProfile(PROTO_BYTES)
        assert profile.get_user_id() == USER_ID
        assert profile.get_username() == USERNAME
        assert profile.get_email() == EMAIL
        # Second pass
        assert profile.get_user_id() == USER_ID

    def test_lazy_user_profile_get_user_id_type(self):
        """get_user_id() returns an int (not str or bytes)."""
        profile = LazyUserProfile(PROTO_BYTES)
        assert isinstance(profile.get_user_id(), int)

    def test_lazy_user_profile_get_username_type(self):
        """get_username() returns a str."""
        profile = LazyUserProfile(PROTO_BYTES)
        assert isinstance(profile.get_username(), str)

    def test_lazy_user_profile_get_email_type(self):
        """get_email() returns a str."""
        profile = LazyUserProfile(PROTO_BYTES)
        assert isinstance(profile.get_email(), str)


# ===========================================================================
# 5–8: Protobuf → Thrift Compact transpilation
# ===========================================================================

class TestTranspileProtobufToThriftCompact:
    """Tests for LazyUserProfile.transpile_protobuf_to_thrift_compact."""

    def _transpile(self, proto: bytes = PROTO_BYTES) -> bytes:
        return LazyUserProfile.transpile_protobuf_to_thrift_compact(proto)

    def test_transpile_protobuf_to_thrift_compact_returns_bytes(self):
        """transpile_protobuf_to_thrift_compact() must return bytes."""
        output = self._transpile()
        assert isinstance(output, bytes)

    def test_transpile_ends_with_stop_byte(self):
        """Thrift Compact encoding must end with STOP byte 0x00."""
        output = self._transpile()
        assert len(output) >= 1, "output must be non-empty"
        assert output[-1] == 0x00, (
            f"Expected STOP byte 0x00 at end, got 0x{output[-1]:02X}"
        )

    def test_transpile_contains_string_fields(self):
        """Both username and email strings must appear verbatim in output."""
        output = self._transpile()
        assert b"zero_copy_hero" in output, (
            "Expected username 'zero_copy_hero' to appear in Thrift output"
        )
        assert b"hero@helix.rpc" in output, (
            "Expected email 'hero@helix.rpc' to appear in Thrift output"
        )

    def test_transpile_roundtrip_different_values(self):
        """Transpile a different user's proto; both strings must appear in output."""
        proto = _build_proto(99, "roundtrip", "rt@test.com")
        output = LazyUserProfile.transpile_protobuf_to_thrift_compact(proto)
        assert isinstance(output, bytes)
        assert b"roundtrip" in output, (
            "Expected username 'roundtrip' to appear in Thrift output"
        )
        assert b"rt@test.com" in output, (
            "Expected email 'rt@test.com' to appear in Thrift output"
        )

    def test_transpile_output_non_empty(self):
        """Output of transpilation must be non-empty bytes."""
        output = self._transpile()
        assert len(output) > 0

    def test_transpile_user_id_zero(self):
        """Transpiling user_id=0 should still produce valid output ending with 0x00."""
        proto = _build_proto(0, "nobody", "nobody@helix.rpc")
        output = LazyUserProfile.transpile_protobuf_to_thrift_compact(proto)
        assert isinstance(output, bytes)
        assert output[-1] == 0x00
        assert b"nobody" in output

    def test_transpile_large_user_id(self):
        """Large user_id (multi-byte varint) transpiles without error."""
        proto = _build_proto(2**32 - 1, "bigid", "big@helix.rpc")
        output = LazyUserProfile.transpile_protobuf_to_thrift_compact(proto)
        assert isinstance(output, bytes)
        assert output[-1] == 0x00
        assert b"bigid" in output

    def test_transpile_unicode_strings(self):
        """Unicode strings survive the proto→Thrift roundtrip intact."""
        proto = _build_proto(7, "hélix", "hélix@rpc.io")
        output = LazyUserProfile.transpile_protobuf_to_thrift_compact(proto)
        assert b"h\xc3\xa9lix" in output  # UTF-8 encoding of 'hélix'


# ===========================================================================
# 9: UserProfile.from_proto classmethod
# ===========================================================================

class TestUserProfileFromProto:
    """Tests for UserProfile.from_proto (zero-copy protobuf decode)."""

    def test_from_proto_classmethod(self):
        """from_proto() must return a UserProfile with correct field values."""
        profile = UserProfile.from_proto(PROTO_BYTES)
        assert isinstance(profile, UserProfile)
        assert profile.user_id == USER_ID
        assert profile.username == USERNAME
        assert profile.email == EMAIL

    def test_from_proto_returns_user_profile_instance(self):
        """from_proto() result must be an instance of UserProfile."""
        profile = UserProfile.from_proto(PROTO_BYTES)
        assert isinstance(profile, UserProfile)

    def test_from_proto_user_id_type(self):
        """from_proto(): user_id field must be an int."""
        profile = UserProfile.from_proto(PROTO_BYTES)
        assert isinstance(profile.user_id, int)

    def test_from_proto_username_type(self):
        """from_proto(): username field must be a str."""
        profile = UserProfile.from_proto(PROTO_BYTES)
        assert isinstance(profile.username, str)

    def test_from_proto_email_type(self):
        """from_proto(): email field must be a str."""
        profile = UserProfile.from_proto(PROTO_BYTES)
        assert isinstance(profile.email, str)

    def test_from_proto_different_payload(self):
        """from_proto() works correctly with different payload values."""
        proto = _build_proto(99, "roundtrip", "rt@test.com")
        profile = UserProfile.from_proto(proto)
        assert profile.user_id == 99
        assert profile.username == "roundtrip"
        assert profile.email == "rt@test.com"

    def test_from_proto_and_lazy_agree(self):
        """UserProfile.from_proto and LazyUserProfile return identical values."""
        eager = UserProfile.from_proto(PROTO_BYTES)
        lazy = LazyUserProfile(PROTO_BYTES)
        assert eager.user_id == lazy.get_user_id()
        assert eager.username == lazy.get_username()
        assert eager.email == lazy.get_email()


# ===========================================================================
# 10–12: eBPF / UDS helper functions
# ===========================================================================

class TestEbpfHelpers:
    """Tests for load_bpf_sockmap, has_unix_prefix, strip_unix_prefix."""

    # ------------------------------------------------------------------
    # load_bpf_sockmap
    # ------------------------------------------------------------------

    def test_ebpf_load_bpf_sockmap_fallback(self):
        """load_bpf_sockmap should return False on non-Linux or non-root environments."""
        result = load_bpf_sockmap("127.0.0.1:9090")
        # On macOS / non-root the function gracefully returns False
        assert isinstance(result, bool), "load_bpf_sockmap must return a bool"
        # On a typical macOS CI runner we expect False; on Linux-as-root it
        # would return True — we tolerate both, but must be a bool.

    def test_ebpf_load_bpf_sockmap_non_loopback_returns_false(self):
        """load_bpf_sockmap with a non-loopback address must return False."""
        result = load_bpf_sockmap("10.0.0.1:8080")
        # Non-loopback is always rejected; result must be bool
        assert isinstance(result, bool)
        # On non-Linux the platform check fires first, still False
        # On Linux-as-root the loopback check fires and returns False
        assert result is False

    def test_ebpf_load_bpf_sockmap_return_type(self):
        """load_bpf_sockmap always returns a plain bool, never None or int."""
        result = load_bpf_sockmap("127.0.0.1:9090")
        assert isinstance(result, bool)
        assert result is not None

    # ------------------------------------------------------------------
    # has_unix_prefix
    # ------------------------------------------------------------------

    def test_ebpf_has_unix_prefix_true(self):
        """has_unix_prefix returns True for 'unix://...' addresses."""
        assert has_unix_prefix("unix:///tmp/helix.sock") is True

    def test_ebpf_has_unix_prefix_false(self):
        """has_unix_prefix returns False for TCP addresses."""
        assert has_unix_prefix("127.0.0.1:9090") is False

    def test_ebpf_has_unix_prefix_false_for_empty(self):
        """has_unix_prefix returns False for an empty string."""
        assert has_unix_prefix("") is False

    def test_ebpf_has_unix_prefix_false_for_http(self):
        """has_unix_prefix returns False for http:// URLs."""
        assert has_unix_prefix("http://localhost:8080") is False

    def test_ebpf_has_unix_prefix_double_slash_only(self):
        """has_unix_prefix requires exactly 'unix://' prefix (7 chars)."""
        assert has_unix_prefix("unix://") is True  # prefix matches
        assert has_unix_prefix("unix:/") is False   # only 6 chars match

    # ------------------------------------------------------------------
    # strip_unix_prefix
    # ------------------------------------------------------------------

    def test_ebpf_strip_unix_prefix(self):
        """strip_unix_prefix removes 'unix://' and returns the path."""
        result = strip_unix_prefix("unix:///tmp/helix.sock")
        assert result == "/tmp/helix.sock"

    def test_ebpf_strip_unix_prefix_no_prefix_passthrough(self):
        """strip_unix_prefix returns the address unchanged if no unix:// prefix."""
        addr = "127.0.0.1:9090"
        assert strip_unix_prefix(addr) == addr

    def test_ebpf_strip_unix_prefix_short_path(self):
        """strip_unix_prefix handles a minimal 'unix:///a' path."""
        assert strip_unix_prefix("unix:///a") == "/a"

    def test_ebpf_strip_unix_prefix_result_type(self):
        """strip_unix_prefix always returns str."""
        result = strip_unix_prefix("unix:///var/run/helix.sock")
        assert isinstance(result, str)

    def test_ebpf_strip_unix_prefix_empty_string(self):
        """strip_unix_prefix with empty string returns empty string (no prefix)."""
        assert strip_unix_prefix("") == ""


# ===========================================================================
# Bonus: Internal helper function unit tests
# ===========================================================================

class TestInternalHelpers:
    """Unit tests for low-level varint / scan helpers exposed by generated_models."""

    def test_read_varint_single_byte(self):
        """_read_varint correctly decodes a single-byte varint (< 128)."""
        buf = memoryview(bytes([42]))
        value, next_idx = _read_varint(buf, 0)
        assert value == 42
        assert next_idx == 1

    def test_read_varint_multi_byte(self):
        """_read_varint correctly decodes a two-byte varint (e.g., 300 = 0xAC 0x02)."""
        buf = memoryview(bytes([0xAC, 0x02]))  # 300 in LEB128
        value, next_idx = _read_varint(buf, 0)
        assert value == 300
        assert next_idx == 2

    def test_write_thrift_varint_single_byte(self):
        """_write_thrift_varint encodes small values as a single byte."""
        out = bytearray()
        _write_thrift_varint(out, 5)
        assert out == bytearray([5])

    def test_write_thrift_varint_multi_byte(self):
        """_write_thrift_varint encodes 300 as two bytes."""
        out = bytearray()
        _write_thrift_varint(out, 300)
        assert out == bytearray([0xAC, 0x02])

    def test_fast_scan_field_finds_string_field(self):
        """_fast_scan_field returns the raw bytes of a string field without full decode."""
        raw = memoryview(PROTO_BYTES)
        data, wire_type = _fast_scan_field(raw, 2)  # field 2 = username
        assert bytes(data) == USERNAME.encode("utf-8")
        assert wire_type == 2  # length-delimited

    def test_fast_scan_field_finds_varint_field(self):
        """_fast_scan_field returns the raw varint bytes for field 1 (user_id)."""
        raw = memoryview(PROTO_BYTES)
        data, wire_type = _fast_scan_field(raw, 1)  # field 1 = user_id
        value, _ = _read_varint(data, 0)
        assert value == USER_ID
        assert wire_type == 0  # varint

    def test_fast_scan_field_missing_field_raises(self):
        """_fast_scan_field raises KeyError when the requested field is absent."""
        raw = memoryview(PROTO_BYTES)
        with pytest.raises(KeyError):
            _fast_scan_field(raw, 99)  # field 99 does not exist

if __name__ == '__main__':
    # Run all classes starting with Test
    import unittest
    # Adapt pytest Test classes to unittest
    suite = unittest.TestSuite()
    loader = unittest.TestLoader()
    
    # We can inspect globals and load test classes
    for name, obj in list(globals().items()):
        if name.startswith("Test") and isinstance(obj, type):
            # Create a dynamic subclass of unittest.TestCase
            class_dict = {}
            for attr in dir(obj):
                if not attr.startswith("__"):
                    class_dict[attr] = getattr(obj, attr)
            tc_class = type(name, (unittest.TestCase,), class_dict)
            suite.addTests(loader.loadTestsFromTestCase(tc_class))
            
    runner = unittest.TextTestRunner(verbosity=2)
    result = runner.run(suite)
    sys.exit(0 if result.wasSuccessful() else 1)


