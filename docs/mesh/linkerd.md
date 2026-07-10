# Linkerd & eBPF Compatibility Guide

Helix RPC supports high-performance zero-copy loopback routing by bypassing the Linux TCP/IP stack using eBPF Sockmaps. 

However, in modern Service Meshes like **Linkerd**, a transparent `linkerd-proxy` sidecar is injected into every Pod. Linkerd utilizes iptables rules to transparently intercept all inbound and outbound traffic, encrypting it using mutual TLS (mTLS) and emitting golden metrics.

This document explicitly defines how Helix RPC interacts with Linkerd.

## 1. Cross-Node Traffic (mTLS Preserved)

When a Helix RPC client in `Node A` calls a Helix RPC service in `Node B`, the eBPF Sockmap bypass **is not triggered**, because the IP addresses belong to different network interfaces.

In this scenario:
- Traffic flows through the `linkerd-proxy` transparently.
- Linkerd encrypts the traffic using mTLS.
- Traffic arrives at `Node B`, is decrypted by the destination `linkerd-proxy`, and handed to the Helix Server.

**Conclusion**: Helix RPC works flawlessly with Linkerd's transparent mTLS for cross-node service-to-service communication. You only need to add the standard annotation:
```yaml
metadata:
  annotations:
    linkerd.io/inject: enabled
```

## 2. Same-Node Co-located Traffic (eBPF Loopback Bypass)

When a Helix RPC client and server are deployed on the **same Kubernetes Node** (often engineered via Pod Affinity rules), and `enableEBPFBypass: true` is set on the `HelixService` CRD, a unique interaction occurs.

1. The Helix Runtime detects that the target IP belongs to the local host's routing table.
2. The eBPF Sockmap intercepts the socket write *before* it reaches the iptables rules.
3. Data is copied directly from the Client socket to the Server socket in kernel memory space.

**What happens to Linkerd?**
Because the eBPF redirect occurs at the socket level (`BPF_PROG_TYPE_SK_MSG`), the traffic completely bypasses the Linux networking stack (TCP/IP layers, routing, and iptables). 

As a result:
- The `linkerd-proxy` **will not intercept** this traffic.
- The traffic is **not encrypted** using mTLS (because it never leaves the local node memory space).
- Linkerd will **not emit** TCP/HTTP metrics for this specific connection.

### Is this safe?
Yes. Because the traffic never touches the physical network interface or leaves the Node's memory space, it cannot be sniffed externally, mitigating the need for mTLS on that specific hop.

### Managing Metrics
Because Linkerd cannot see the bypassed traffic, your service mesh dashboards will report lower RPS than actuality. 
However, since Helix RPC natively includes **OpenTelemetry SDK Integration**, you can rely on Helix's native OpenTelemetry `SERVER` spans and metrics to accurately capture 100% of the traffic, regardless of whether it was routed over the physical network (mTLS) or the eBPF loopback.
