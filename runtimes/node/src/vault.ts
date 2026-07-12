export interface VaultOptions {
    endpoint: string;
    token?: string;
    appRole?: {
        roleId: string;
        secretId: string;
    };
    secretPath: string;
    refreshIntervalMs?: number;
}

export class HelixVault {
    private vault: any;
    private options: VaultOptions;
    private vaultModule: any;
    private timer?: ReturnType<typeof setInterval>;
    private keys: Record<string, any> = {};

    constructor(options: VaultOptions) {
        this.options = {
            refreshIntervalMs: 60000,
            ...options
        };
    }

    private async init() {
        if (!this.vault) {
            try {
                this.vaultModule = (await import("node-vault")).default;
                this.vault = this.vaultModule({ endpoint: this.options.endpoint });
            } catch (err) {
                console.warn("HelixVault: node-vault not installed.");
                throw new Error("Missing peer dependency node-vault");
            }
        }
    }

    async authenticate() {
        await this.init();
        if (this.options.token) {
            this.vault.token = this.options.token;
        } else if (this.options.appRole) {
            const result = await this.vault.approleLogin({
                role_id: this.options.appRole.roleId,
                secret_id: this.options.appRole.secretId
            });
            this.vault.token = result.auth.client_token;
        } else {
            throw new Error("HelixVault: No authentication method provided");
        }
    }

    async fetchKeys() {
        if (!this.vault || !this.vault.token) {
            await this.authenticate();
        }
        
        try {
            const response = await this.vault.read(this.options.secretPath);
            this.keys = response.data.data || response.data;
        } catch (error) {
            console.error("HelixVault Error fetching keys:", error);
            throw error;
        }
    }

    startAutoRefresh() {
        if (this.timer) {
            clearInterval(this.timer);
        }
        
        // Initial fetch
        this.fetchKeys().catch(console.error);

        this.timer = setInterval(() => {
            this.fetchKeys().catch(console.error);
        }, this.options.refreshIntervalMs);
    }

    stopAutoRefresh() {
        if (this.timer) {
            clearInterval(this.timer);
            this.timer = undefined;
        }
    }

    getKey(name: string): any {
        return this.keys[name];
    }
}
