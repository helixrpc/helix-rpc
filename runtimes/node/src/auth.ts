import * as crypto from 'crypto';

export interface AuthContext {
    claims?: any;
    apiKey?: string;
}

export function verifyApiKey(headerValue: string, expectedKey: string): boolean {
    if (!headerValue || !expectedKey) return false;
    const key = headerValue.startsWith('Bearer ') ? headerValue.slice(7) : headerValue;
    return crypto.timingSafeEqual(Buffer.from(key), Buffer.from(expectedKey));
}

export function decodeAndVerifyJWT(token: string, secret: string): any | null {
    try {
        const parts = token.split('.');
        if (parts.length !== 3) return null;

        const [headerB64, payloadB64, signatureB64] = parts;
        const signature = crypto
            .createHmac('sha256', secret)
            .update(`${headerB64}.${payloadB64}`)
            .digest('base64url');

        if (signature !== signatureB64) {
            return null;
        }

        const payloadStr = Buffer.from(payloadB64, 'base64url').toString('utf8');
        const payload = JSON.parse(payloadStr);

        // Check expiration
        if (payload.exp && payload.exp < Math.floor(Date.now() / 1000)) {
            return null;
        }

        return payload;
    } catch {
        return null;
    }
}
