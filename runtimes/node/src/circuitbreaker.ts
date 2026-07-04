export enum State {
    Closed,
    Open,
    HalfOpen
}

export class CircuitBreaker {
    private state: State = State.Closed;
    private failureCount: number = 0;
    private lastFailureTime: number = 0;
    private maxFailures: number;
    private timeout: number;

    constructor(maxFailures: number = 3, timeoutMs: number = 5000) {
        this.maxFailures = maxFailures;
        this.timeout = timeoutMs;
    }

    public async execute<T>(fn: () => Promise<T>): Promise<T> {
        this.checkState();
        if (this.state === State.Open) {
            throw new Error("circuit breaker is open");
        }

        try {
            const result = await fn();
            this.onSuccess();
            return result;
        } catch (err) {
            this.onFailure();
            throw err;
        }
    }

    private checkState() {
        if (this.state === State.Open && Date.now() - this.lastFailureTime > this.timeout) {
            this.state = State.HalfOpen;
        }
    }

    private onSuccess() {
        this.state = State.Closed;
        this.failureCount = 0;
    }

    private onFailure() {
        this.failureCount++;
        this.lastFailureTime = Date.now();
        if (this.state === State.HalfOpen || this.failureCount >= this.maxFailures) {
            this.state = State.Open;
        }
    }

    public getState(): State {
        this.checkState();
        return this.state;
    }
}
