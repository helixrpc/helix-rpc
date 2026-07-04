export interface Client {
    invoke(path: string, request: any): Promise<any>;
}

export class RoundRobinBalancer {
    private clients: Client[] = [];
    private index: number = 0;

    constructor(clients: Client[] = []) {
        this.clients = clients;
    }

    public register(client: Client) {
        this.clients.push(client);
    }

    public next(): Client {
        if (this.clients.length === 0) {
            throw new Error("no clients registered in balancer");
        }
        const client = this.clients[this.index];
        this.index = (this.index + 1) % this.clients.length;
        return client;
    }
}
