import pytest
from unittest.mock import AsyncMock, patch
from helix_rt.rabbitmq import RabbitMQAsyncSink

@pytest.mark.asyncio
async def test_rabbitmq_publish():
    with patch("helix_rt.rabbitmq.aio_pika") as mock_aio_pika:
        mock_connection = AsyncMock()
        mock_channel = AsyncMock()
        mock_aio_pika.connect_robust = AsyncMock(return_value=mock_connection)
        mock_connection.channel = AsyncMock(return_value=mock_channel)
        
        sink = RabbitMQAsyncSink("amqp://localhost", "my_key")
        await sink.publish_async({"test": "data"})
        
        mock_aio_pika.connect_robust.assert_called_once_with("amqp://localhost")
        mock_connection.channel.assert_called_once()
        mock_channel.default_exchange.publish.assert_called_once()

@pytest.mark.asyncio
async def test_rabbitmq_close():
    sink = RabbitMQAsyncSink("amqp://localhost", "my_key")
    sink.connection = AsyncMock()
    await sink.close()
    sink.connection.close.assert_called_once()
