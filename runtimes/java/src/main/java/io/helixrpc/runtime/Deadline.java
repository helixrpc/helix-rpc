package io.helixrpc.runtime;

import java.time.Duration;

public class Deadline {
    public static Duration parseGRPCTimeout(String timeoutStr) {
        if (timeoutStr == null || timeoutStr.isEmpty()) {
            throw new IllegalArgumentException("empty timeout header");
        }

        int valEnd = 0;
        while (valEnd < timeoutStr.length() && Character.isDigit(timeoutStr.charAt(valEnd))) {
            valEnd++;
        }

        if (valEnd == 0 || valEnd == timeoutStr.length()) {
            throw new IllegalArgumentException("invalid timeout format");
        }

        long value = Long.parseLong(timeoutStr.substring(0, valEnd));
        char unit = timeoutStr.charAt(valEnd);

        switch (unit) {
            case 'n': return Duration.ofNanos(value);
            case 'u': return Duration.ofNanos(value * 1000);
            case 'm': return Duration.ofMillis(value);
            case 'S': return Duration.ofSeconds(value);
            case 'M': return Duration.ofMinutes(value);
            case 'H': return Duration.ofHours(value);
            default:
                throw new IllegalArgumentException("unknown timeout unit");
        }
    }
}
