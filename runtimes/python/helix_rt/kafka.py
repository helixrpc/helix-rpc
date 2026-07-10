from aiokafka import AIOKafkaProducer
import json

class KafkaAsyncSink:
    def __init__(self, bootstrap_servers: str, topic: str):
        self.bootstrap_servers = bootstrap_servers
        self.topic = topic
        self.producer = None

    async def connect(self):
        if not self.producer:
            self.producer = AIOKafkaProducer(
                bootstrap_servers=self.bootstrap_servers,
                value_serializer=lambda v: json.dumps(v).encode('utf-8')
            )
            await self.producer.start()

    async def publish_async(self, message: dict):
        await self.connect()
        await self.producer.send_and_wait(self.topic, message)

    async def close(self):
        if self.producer:
            await self.producer.stop()
