import asyncio
import struct


class QuicVirtualStream:
    def __init__(self, stream_id: int, addr: tuple, transport: asyncio.DatagramTransport):
        self.stream_id = stream_id
        self.addr = addr
        self.transport = transport
        self.queue = asyncio.Queue()

    async def read(self) -> bytes:
        return await self.queue.get()

    def write(self, data: bytes) -> None:
        packet = struct.pack(">I", self.stream_id) + data
        self.transport.sendto(packet, self.addr)


class QuicProtocol(asyncio.DatagramProtocol):
    def __init__(self, listener):
        self.listener = listener

    def connection_made(self, transport):
        self.listener.transport = transport

    def datagram_received(self, data: bytes, addr: tuple):
        if len(data) < 4:
            return
        stream_id = struct.unpack(">I", data[:4])[0]
        key = (addr, stream_id)

        stream = self.listener.streams.get(key)
        if not stream:
            stream = QuicVirtualStream(stream_id, addr, self.listener.transport)
            self.listener.streams[key] = stream
            self.listener.queue.put_nowait(stream)

        stream.queue.put_nowait(data[4:])


class QuicListener:
    def __init__(self, host: str = "127.0.0.1", port: int = 0):
        self.host = host
        self.port = port
        self.streams = {}
        self.queue = asyncio.Queue()
        self.transport = None
        self.protocol = None

    async def start(self):
        loop = asyncio.get_running_loop()
        self.transport, self.protocol = await loop.create_datagram_endpoint(
            lambda: QuicProtocol(self),
            local_addr=(self.host, self.port)
        )
        self.port = self.transport.get_extra_info("sockname")[1]

    async def accept(self) -> QuicVirtualStream:
        return await self.queue.get()

    def close(self):
        if self.transport:
            self.transport.close()
            self.transport = None
