import socket

class Resolver:
    def resolve(self, service_name: str) -> list[str]:
        raise NotImplementedError

class StaticResolver(Resolver):
    def __init__(self):
        self.targets = {}

    def register(self, service_name: str, addresses: list[str]) -> None:
        self.targets[service_name] = addresses

    def resolve(self, service_name: str) -> list[str]:
        if service_name not in self.targets:
            raise ValueError(f"service {service_name} not found in resolver targets")
        return self.targets[service_name]

class DNSResolver(Resolver):
    def resolve(self, service_name: str) -> list[str]:
        if ":" in service_name:
            host, port = service_name.split(":", 1)
            port = int(port)
        else:
            host = service_name
            port = 8080

        try:
            infos = socket.getaddrinfo(host, port, proto=socket.IPPROTO_TCP)
            ips = list(set([f"{info[4][0]}:{port}" for info in infos]))
            if not ips:
                raise ValueError(f"No IP addresses resolved for {service_name}")
            return ips
        except Exception as e:
            raise ValueError(f"Failed to resolve DNS target {service_name}: {e}")
