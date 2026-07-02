export interface RetryOptions {
    maxAttempts: number;
    initialBackoffMs: number;
    maxBackoffMs: number;
    multiplier: number;
}

export async function withRetries<T>(
    fn: () => Promise<T>,
    options: RetryOptions = { maxAttempts: 3, initialBackoffMs: 100, maxBackoffMs: 1000, multiplier: 2 }
): Promise<T> {
    let attempt = 0;
    let delay = options.initialBackoffMs;

    while (true) {
        try {
            attempt++;
            return await fn();
        } catch (err) {
            if (attempt >= options.maxAttempts) {
                throw err;
            }
            // Add full jitter
            const jitterDelay = Math.random() * delay;
            await new Promise(resolve => setTimeout(resolve, jitterDelay));
            delay = Math.min(options.maxBackoffMs, delay * options.multiplier);
        }
    }
}
