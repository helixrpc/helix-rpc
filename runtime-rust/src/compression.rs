use std::io::{Read, Write};

/// Trait for per-message compressors used in gRPC framing.
pub trait Compressor: Send + Sync {
    /// Compression algorithm name (e.g. `"gzip"`).
    fn name(&self) -> &str;

    /// Compress `data` and return the compressed bytes.
    fn compress(&self, data: &[u8]) -> Result<Vec<u8>, String>;

    /// Decompress `data` and return the original bytes.
    fn decompress(&self, data: &[u8]) -> Result<Vec<u8>, String>;
}

/// Gzip compressor backed by `flate2`.
pub struct GzipCompressor;

impl Compressor for GzipCompressor {
    fn name(&self) -> &str {
        "gzip"
    }

    fn compress(&self, data: &[u8]) -> Result<Vec<u8>, String> {
        use flate2::write::GzEncoder;
        use flate2::Compression;

        let mut encoder = GzEncoder::new(Vec::new(), Compression::default());
        encoder
            .write_all(data)
            .map_err(|e| format!("gzip compress: {}", e))?;
        encoder
            .finish()
            .map_err(|e| format!("gzip compress finish: {}", e))
    }

    fn decompress(&self, data: &[u8]) -> Result<Vec<u8>, String> {
        use flate2::read::GzDecoder;

        let mut decoder = GzDecoder::new(data);
        let mut out = Vec::new();
        decoder
            .read_to_end(&mut out)
            .map_err(|e| format!("gzip decompress: {}", e))?;
        Ok(out)
    }
}

/// Look up a compressor by name.  Currently only `"gzip"` is supported.
pub fn get_compressor(name: &str) -> Option<Box<dyn Compressor>> {
    match name {
        "gzip" => Some(Box::new(GzipCompressor)),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_gzip_roundtrip() {
        let original = b"hello world, this is a test of gzip compression!";
        let c = GzipCompressor;
        let compressed = c.compress(original).unwrap();
        let decompressed = c.decompress(&compressed).unwrap();
        assert_eq!(decompressed, original);
    }

    #[test]
    fn test_get_compressor() {
        assert!(get_compressor("gzip").is_some());
        assert!(get_compressor("deflate").is_none());
        assert!(get_compressor("").is_none());
    }
}
