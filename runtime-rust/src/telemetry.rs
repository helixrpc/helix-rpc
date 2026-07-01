use hyper::HeaderMap;
use opentelemetry::global;
use opentelemetry::propagation::Extractor;
use tracing_opentelemetry::OpenTelemetrySpanExt;
use tracing::Span;

struct HeaderExtractor<'a>(&'a HeaderMap);

impl<'a> Extractor for HeaderExtractor<'a> {
    fn get(&self, key: &str) -> Option<&str> {
        self.0.get(key).and_then(|v| v.to_str().ok())
    }

    fn keys(&self) -> Vec<&str> {
        self.0.keys().map(|k| k.as_str()).collect()
    }
}

/// Sampling strategy for distributed tracing.
#[derive(Clone, Copy, Debug)]
pub enum SamplingStrategy {
    /// Record every request — use only in development.
    All,
    /// Record no spans — disables telemetry.
    None,
    /// Record a random fraction of requests (e.g. 0.01 = 1%).
    Probabilistic(f64),
}

impl SamplingStrategy {
    /// Returns true if this request should be sampled.
    pub fn should_sample(&self) -> bool {
        match self {
            SamplingStrategy::All => true,
            SamplingStrategy::None => false,
            SamplingStrategy::Probabilistic(rate) => {
                // Fast thread-local random — no locking needed.
                rand::random::<f64>() < *rate
            }
        }
    }
}

/// Extracts OpenTelemetry context from hyper headers and attaches it to the
/// current span. Respects `strategy` — if the current request is not sampled,
/// this is a no-op.
pub fn attach_span_context(headers: &HeaderMap, span: &Span, strategy: SamplingStrategy) {
    if !strategy.should_sample() {
        return;
    }
    let extractor = HeaderExtractor(headers);
    let parent_cx = global::get_text_map_propagator(|prop| prop.extract(&extractor));
    let _ = span.set_parent(parent_cx);
}
