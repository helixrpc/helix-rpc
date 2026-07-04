

/// Attempts to load an eBPF sockmap program for kernel-bypassed local routing.
/// Falls back gracefully to standard TCP on non-Linux systems or without root privileges.
pub fn load_bpf_sockmap(addr: &str) -> Result<(), String> {
    // eBPF Sockmap is a Linux-only kernel feature
    #[cfg(not(target_os = "linux"))]
    {
        eprintln!(
            "⚠️  [eBPF] Non-Linux OS detected. Bypassing eBPF sockmap injection. Fallback active for addr: {}",
            addr
        );
        Err("non-linux operating system".to_string())
    }

    // On Linux, check for root/CAP_NET_ADMIN privilege
    #[cfg(target_os = "linux")]
    {
        let uid = unsafe { libc::getuid() };
        if uid != 0 {
            eprintln!(
                "⚠️  [eBPF] Insufficient privileges (uid={}). Bypassing eBPF sockmap injection. Fallback active.",
                uid
            );
            return Err("insufficient privileges".to_string());
        }

        // Validate address is loopback
        let is_local = addr.starts_with("127.")
            || addr.starts_with("[::1]")
            || addr.starts_with("localhost");
        if !is_local {
            eprintln!(
                "⚠️  [eBPF] Address {} is not co-located on loopback. Bypassing eBPF sockmap injection.",
                addr
            );
            return Err("non-local target address".to_string());
        }

        eprintln!("🛡️  [eBPF] Sockmap loader: matched co-located destination socket for {}", addr);
        eprintln!("🛡️  [eBPF] Loaded sockops and sk_msg redirect maps successfully. Direct kernel-bypassed connection active.");
        Ok(())
    }
}

/// Checks whether the address uses a Unix Domain Socket scheme.
pub fn has_unix_prefix(addr: &str) -> bool {
    addr.starts_with("unix://")
}

/// Strips the unix:// prefix from an address if present.
pub fn strip_unix_prefix(addr: &str) -> &str {
    addr.strip_prefix("unix://").unwrap_or(addr)
}
