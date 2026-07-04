import * as crypto from 'crypto';

export interface LogFields {
    timestamp: string;
    level: string;
    message: string;
    traceId?: string;
    latencyMs?: number;
    method?: string;
    [key: string]: any;
}

export function logStructured(level: string, message: string, extra: Record<string, any> = {}) {
    const fields: LogFields = {
        timestamp: new Date().toISOString(),
        level,
        message,
        ...extra
    };
    console.log(JSON.stringify(fields));
}

export function getOrCreateTraceId(headers: Record<string, string | string[] | undefined>): string {
    const headerVal = headers['x-trace-id'];
    if (headerVal) {
        return Array.isArray(headerVal) ? headerVal[0] : headerVal;
    }
    return crypto.randomBytes(16).toString('hex');
}
