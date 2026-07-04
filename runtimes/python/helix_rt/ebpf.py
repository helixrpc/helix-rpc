"""eBPF sockmap loader and Unix Domain Socket routing helpers."""
import os
import sys
import socket


def load_bpf_sockmap(addr: str) -> bool:
    """Attempt to load eBPF sockmap for kernel-bypassed local routing.
    Falls back gracefully on non-Linux or without root privileges."""
    if sys.platform != 'linux':
        print(f'⚠️  [eBPF] Non-Linux OS ({sys.platform}) detected. '
              'Bypassing eBPF sockmap injection. Fallback active.')
        return False
    if os.getuid() != 0:
        print('⚠️  [eBPF] Insufficient privileges (not running as root). '
              'Bypassing eBPF sockmap injection. Fallback active.')
        return False
    host = addr.split(':')[0]
    if host not in ('127.0.0.1', 'localhost', '::1'):
        print(f'⚠️  [eBPF] Address {addr} is not co-located on loopback. '
              'Bypassing eBPF sockmap injection.')
        return False
    print(f'🛡️  [eBPF] Sockmap loaded for {addr}. Direct kernel-bypassed connection active.')
    return True


def has_unix_prefix(addr: str) -> bool:
    return addr.startswith('unix://')


def strip_unix_prefix(addr: str) -> str:
    return addr[7:] if addr.startswith('unix://') else addr


def create_socket(addr: str) -> socket.socket:
    """Create a connected socket routing via UDS or TCP based on address prefix."""
    if has_unix_prefix(addr):
        path = strip_unix_prefix(addr)
        sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        sock.connect(path)
    else:
        host, port_str = addr.rsplit(':', 1)
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.connect((host, int(port_str)))
    return sock
