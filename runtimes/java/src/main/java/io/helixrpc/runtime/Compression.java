package io.helixrpc.runtime;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.util.zip.GZIPInputStream;
import java.util.zip.GZIPOutputStream;

public class Compression {
    public interface Compressor {
        String name();
        byte[] compress(byte[] input) throws IOException;
        byte[] decompress(byte[] input) throws IOException;
    }

    public static class GzipCompressor implements Compressor {
        @Override
        public String name() {
            return "gzip";
        }

        @Override
        public byte[] compress(byte[] input) throws IOException {
            ByteArrayOutputStream bos = new ByteArrayOutputStream();
            try (GZIPOutputStream gzos = new GZIPOutputStream(bos)) {
                gzos.write(input);
            }
            return bos.toByteArray();
        }

        @Override
        public byte[] decompress(byte[] input) throws IOException {
            ByteArrayInputStream bis = new ByteArrayInputStream(input);
            ByteArrayOutputStream bos = new ByteArrayOutputStream();
            try (GZIPInputStream gzis = new GZIPInputStream(bis)) {
                byte[] buf = new byte[1024];
                int len;
                while ((len = gzis.read(buf)) > 0) {
                    bos.write(buf, 0, len);
                }
            }
            return bos.toByteArray();
        }
    }
}
