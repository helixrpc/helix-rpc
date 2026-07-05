import asyncio
import os


class MultiplexedServer:
    def __init__(self, host: str = "127.0.0.1", port: int = 0):
        self.host = host
        self.port = port
        self.server = None

    async def start(self, grpc_handler, http_handler):
        if self.host.startswith("unix://"):
            path = self.host[7:]
            try:
                os.unlink(path)
            except OSError:
                pass
            self.server = await asyncio.start_unix_server(
                lambda r, w: self.handle_connection(r, w, grpc_handler, http_handler),
                path
            )
            os.chmod(path, 0o600)
            self.port = 0
        else:
            self.server = await asyncio.start_server(
                lambda r, w: self.handle_connection(r, w, grpc_handler, http_handler),
                self.host,
                self.port
            )
            self.port = self.server.sockets[0].getsockname()[1]

    async def handle_connection(self, reader, writer, grpc_handler, http_handler):
        try:
            # Peek connection protocol
            peek_bytes = await reader.read(8)
            if len(peek_bytes) >= 4 and peek_bytes[:4] == b"PRI ":
                # gRPC protocol
                await grpc_handler(reader, writer, peek_bytes)
            else:
                # HTTP/REST protocol
                await http_handler(reader, writer, peek_bytes)
        except Exception:
            pass
        finally:
            writer.close()
            await writer.wait_closed()

    async def stop(self):
        if self.server:
            self.server.close()
            await self.server.wait_closed()


async def write_sse_chunk(writer, data: str) -> None:
    """Helper to write SSE stream chunks to a connection writer."""
    chunk = f"data: {data}\n\n".encode("utf-8")
    writer.write(chunk)
    await writer.drain()
