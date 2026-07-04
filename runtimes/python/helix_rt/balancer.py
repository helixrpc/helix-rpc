class RoundRobinBalancer:
    def __init__(self, clients=None):
        self.clients = clients or []
        self.index = 0

    def register(self, client):
        self.clients.append(client)

    def next(self):
        if not self.clients:
            raise Exception("no clients registered in balancer")
        client = self.clients[self.index]
        self.index = (self.index + 1) % len(self.clients)
        return client
