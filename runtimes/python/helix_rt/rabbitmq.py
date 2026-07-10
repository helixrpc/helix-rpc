import aio_pika
import json

class RabbitMQAsyncSink:
    def __init__(self, connection_url: str, routing_key: str):
        self.connection_url = connection_url
        self.routing_key = routing_key
        self.connection = None
        self.channel = None

    async def connect(self):
        if not self.connection:
            self.connection = await aio_pika.connect_robust(self.connection_url)
            self.channel = await self.connection.channel()

    async def publish_async(self, message: dict):
        await self.connect()
        await self.channel.default_exchange.publish(
            aio_pika.Message(body=json.dumps(message).encode('utf-8')),
            routing_key=self.routing_key,
        )

    async def close(self):
        if self.connection:
            await self.connection.close()
