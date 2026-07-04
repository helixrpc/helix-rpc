import * as os from 'os';
import * as net from 'net';

/**
 * Attempts to load eBPF sockmap for kernel-bypassed local routing.
 * On non-Linux systems or without root, logs a warning and returns false.
 */
export function loadBpfSockmap(addr: string): boolean {
    if (os.platform() !== 'linux') {
        console.warn(`⚠️  [eBPF] Non-Linux OS (${os.platform()}) detected. Bypassing eBPF sockmap injection. Fallback active.`);
        return false;
    }
    if (process.getuid && process.getuid() !== 0) {
        console.warn('⚠️  [eBPF] Insufficient privileges (not running as root). Bypassing eBPF sockmap injection. Fallback active.');
        return false;
    }
    // Parse host from addr
    const host = addr.split(':')[0];
    if (host !== '127.0.0.1' && host !== 'localhost' && host !== '::1') {
        console.warn(`⚠️  [eBPF] Address ${addr} is not co-located on loopback. Bypassing eBPF sockmap injection.`);
        return false;
    }
    console.log(`🛡️  [eBPF] Sockmap loaded for ${addr}. Direct kernel-bypassed connection active.`);
    return true;
}

export function hasUnixPrefix(addr: string): boolean {
    return addr.startsWith('unix://');
}

export function stripUnixPrefix(addr: string): string {
    return addr.startsWith('unix://') ? addr.slice(7) : addr;
}

/**
 * Creates a net.Socket connected via Unix Domain Socket or TCP,
 * automatically routing based on address prefix.
 */
export function createSocket(addr: string): Promise<net.Socket> {
    return new Promise((resolve, reject) => {
        const socket = new net.Socket();
        socket.on('error', reject);
        if (hasUnixPrefix(addr)) {
            const path = stripUnixPrefix(addr);
            socket.connect({ path }, () => resolve(socket));
        } else {
            const [host, portStr] = addr.split(':');
            socket.connect({ host, port: parseInt(portStr, 10) }, () => resolve(socket));
        }
    });
}
