package io.helixrpc.runtime.crypto;

import org.springframework.vault.core.VaultTemplate;
import org.springframework.vault.support.VaultResponse;

import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

public class HelixVault {
    private final VaultTemplate vaultTemplate;
    private final String path;
    private final ScheduledExecutorService executorService;
    private final AtomicReference<String> cachedKey;

    public HelixVault(VaultTemplate vaultTemplate, String path, long initialDelay, long period, TimeUnit unit) {
        this.vaultTemplate = vaultTemplate;
        this.path = path;
        this.cachedKey = new AtomicReference<>();
        this.executorService = Executors.newSingleThreadScheduledExecutor();
        this.executorService.scheduleAtFixedRate(this::reloadKey, initialDelay, period, unit);
    }

    private void reloadKey() {
        try {
            VaultResponse response = vaultTemplate.read(path);
            if (response != null && response.getData() != null) {
                Object key = response.getData().get("key");
                if (key != null) {
                    cachedKey.set(key.toString());
                }
            }
        } catch (Exception e) {
            // Handle error silently for background thread
            System.err.println("Failed to reload key from Vault: " + e.getMessage());
        }
    }

    public String getCurrentKey() {
        return cachedKey.get();
    }

    public void stop() {
        executorService.shutdown();
    }
}
