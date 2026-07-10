import pytest
from unittest.mock import AsyncMock, patch
from helix_rt.kafka import KafkaAsyncSink

@pytest.mark.asyncio
async def test_kafka_publish():
    with patch("helix_rt.kafka.AIOKafkaProducer") as mock_producer_class:
        mock_producer = AsyncMock()
        mock_producer_class.return_value = mock_producer
        
        sink = KafkaAsyncSink("localhost:9092", "my_topic")
        await sink.publish_async({"test": "data"})
        
        mock_producer_class.assert_called_once()
        mock_producer.start.assert_called_once()
        mock_producer.send_and_wait.assert_called_once_with("my_topic", {"test": "data"})

@pytest.mark.asyncio
async def test_kafka_close():
    sink = KafkaAsyncSink("localhost:9092", "my_topic")
    sink.producer = AsyncMock()
    await sink.close()
    sink.producer.stop.assert_called_once()
