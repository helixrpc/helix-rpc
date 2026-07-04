export interface BatchJob<Req, Resp> {
    req: Req;
    resolve: (value: Resp) => void;
    reject: (reason: any) => void;
}

export class BatchScheduler<Req, Resp> {
    private maxBatchSize: number;
    private maxDelayMs: number;
    private queue: BatchJob<Req, Resp>[] = [];
    private timer: NodeJS.Timeout | null = null;
    private batchHandler: (batch: Req[]) => Promise<Resp[]>;

    constructor(
        batchHandler: (batch: Req[]) => Promise<Resp[]>,
        maxBatchSize: number = 10,
        maxDelayMs: number = 10
    ) {
        this.batchHandler = batchHandler;
        this.maxBatchSize = maxBatchSize;
        this.maxDelayMs = maxDelayMs;
    }

    public async submit(req: Req): Promise<Resp> {
        return new Promise<Resp>((resolve, reject) => {
            this.queue.push({ req, resolve, reject });
            if (this.queue.length >= this.maxBatchSize) {
                this.flush();
            } else if (!this.timer) {
                this.timer = setTimeout(() => this.flush(), this.maxDelayMs);
            }
        });
    }

    private async flush() {
        if (this.timer) {
            clearTimeout(this.timer);
            this.timer = null;
        }

        const batch = this.queue;
        this.queue = [];
        if (batch.length === 0) return;

        try {
            const requests = batch.map(job => job.req);
            const responses = await this.batchHandler(requests);

            for (let i = 0; i < batch.length; i++) {
                if (i < responses.length) {
                    batch[i].resolve(responses[i]);
                } else {
                    batch[i].reject(new Error("No response returned for batch item"));
                }
            }
        } catch (err) {
            for (const job of batch) {
                job.reject(err);
            }
        }
    }
}
