use std::collections::HashMap;
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::Arc;
use tokio::sync::{oneshot, RwLock};

/// Dynamic batch request
struct BatchRequest {
    payload: Vec<u8>,
    is_json: bool,
    tx: oneshot::Sender<Result<(Vec<u8>, String), String>>,
}

/// BatchScheduler collects concurrent requests into a single dispatch batch.
/// When the batch window expires OR max_size is reached, all pending requests
/// are dispatched together to the handler and results are fanned back out.
pub struct BatchScheduler {
    tx: tokio::sync::mpsc::Sender<BatchRequest>,
}

impl BatchScheduler {
    pub fn new<H>(max_size: usize, window_ms: u64, handler: H) -> Self
    where
        H: Fn(Vec<Vec<u8>>, bool) -> Result<Vec<Vec<u8>>, String> + Send + Sync + 'static,
    {
        let (tx, mut rx) = tokio::sync::mpsc::channel::<BatchRequest>(max_size * 100);
        let handler = Arc::new(handler);

        tokio::spawn(async move {
            let mut batch: Vec<BatchRequest> = Vec::new();

            loop {
                // Wait for the first item
                match rx.recv().await {
                    None => break,
                    Some(req) => {
                        let is_json = req.is_json;
                        batch.push(req);

                        // Drain more items up to max_size within the window
                        let window = tokio::time::Duration::from_millis(window_ms);
                        let deadline = tokio::time::Instant::now() + window;

                        loop {
                            if batch.len() >= max_size {
                                break;
                            }
                            match tokio::time::timeout_at(deadline, rx.recv()).await {
                                Ok(Some(r)) => batch.push(r),
                                _ => break,
                            }
                        }

                        // Dispatch the batch
                        let payloads: Vec<Vec<u8>> =
                            batch.iter().map(|r| r.payload.clone()).collect();
                        let h = handler.clone();
                        let results =
                            tokio::task::spawn_blocking(move || h(payloads, is_json)).await;

                        match results {
                            Ok(Ok(resps)) if resps.len() == batch.len() => {
                                for (req, resp) in batch.drain(..).zip(resps) {
                                    let _ = req.tx.send(Ok((
                                        resp,
                                        if is_json {
                                            "application/json".to_string()
                                        } else {
                                            "application/grpc".to_string()
                                        },
                                    )));
                                }
                            }
                            Ok(Ok(_)) => {
                                for req in batch.drain(..) {
                                    let _ = req.tx.send(Err(
                                        "batch handler returned mismatched response count"
                                            .to_string(),
                                    ));
                                }
                            }
                            Ok(Err(e)) => {
                                for req in batch.drain(..) {
                                    let _ = req.tx.send(Err(e.clone()));
                                }
                            }
                            Err(e) => {
                                let msg = e.to_string();
                                for req in batch.drain(..) {
                                    let _ = req.tx.send(Err(msg.clone()));
                                }
                            }
                        }
                    }
                }
            }
        });

        BatchScheduler { tx }
    }

    pub async fn invoke(
        &self,
        payload: Vec<u8>,
        is_json: bool,
    ) -> Result<(Vec<u8>, String), String> {
        let (tx, rx) = oneshot::channel();
        self.tx
            .send(BatchRequest {
                payload,
                is_json,
                tx,
            })
            .await
            .map_err(|e| e.to_string())?;
        rx.await.map_err(|e| e.to_string())?
    }
}

/// LeastConnBalancer is a Rust port of the lock-free Go implementation.
/// It uses a RwLock-protected pre-indexed map with AtomicI64 counters per target.
pub struct LeastConnBalancer {
    endpoints: Arc<RwLock<HashMap<String, Arc<AtomicI64>>>>,
}

impl LeastConnBalancer {
    pub fn new() -> Self {
        LeastConnBalancer {
            endpoints: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    pub async fn register(&self, targets: &[&str]) {
        let mut map = self.endpoints.write().await;
        for t in targets {
            map.entry(t.to_string())
                .or_insert_with(|| Arc::new(AtomicI64::new(0)));
        }
    }

    pub async fn next(&self, targets: &[String]) -> Result<String, String> {
        if targets.is_empty() {
            return Err("no targets available".to_string());
        }

        let map = self.endpoints.read().await;

        // Ensure all targets registered (register new ones via write lock if needed)
        let mut selected = targets[0].clone();
        let mut min_active = i64::MAX;

        for t in targets {
            let counter = map.get(t.as_str());
            let active = counter.map(|c| c.load(Ordering::Relaxed)).unwrap_or(0);
            if active < min_active {
                min_active = active;
                selected = t.clone();
            }
        }

        // Increment winner
        if let Some(counter) = map.get(selected.as_str()) {
            counter.fetch_add(1, Ordering::Relaxed);
        }

        Ok(selected)
    }

    pub async fn done(&self, target: &str) {
        let map = self.endpoints.read().await;
        if let Some(counter) = map.get(target) {
            counter.fetch_sub(1, Ordering::Relaxed);
        }
    }
}

impl Default for LeastConnBalancer {
    fn default() -> Self {
        Self::new()
    }
}
