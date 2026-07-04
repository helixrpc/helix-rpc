class Message:
    def __init__(self, key: bytes, value: bytes, topic: str, timestamp: int = 0):
        self.key = key
        self.value = value
        self.topic = topic
        self.timestamp = timestamp

class EventProducer:
    """Interface wrapper for dispatching streaming events (Kafka/RabbitMQ/etc.)."""
    async def produce(self, message: Message) -> None:
        raise NotImplementedError

    async def close(self) -> None:
        raise NotImplementedError

class EventConsumer:
    """Interface wrapper for subscribing to event streaming topics (Kafka/RabbitMQ/etc.)."""
    async def consume(self, topic: str, handler) -> None:
        raise NotImplementedError

    async def close(self) -> None:
        raise NotImplementedError
