import { RabbitMQAsyncSink } from './rabbitmq';
import * as amqp from 'amqplib';

jest.mock('amqplib');

describe('RabbitMQAsyncSink', () => {
    let sink: RabbitMQAsyncSink;
    let mockConnection: { createChannel: jest.Mock; close: jest.Mock };
    let mockChannel: { assertQueue: jest.Mock; sendToQueue: jest.Mock; close: jest.Mock };

    beforeEach(() => {
        jest.clearAllMocks();
        mockChannel = {
            assertQueue: jest.fn().mockResolvedValue({}),
            sendToQueue: jest.fn(),
            close: jest.fn().mockResolvedValue(undefined),
        };

        mockConnection = {
            createChannel: jest.fn().mockResolvedValue(mockChannel),
            close: jest.fn().mockResolvedValue(undefined),
        };

        (amqp.connect as jest.Mock).mockResolvedValue(mockConnection);
        
        sink = new RabbitMQAsyncSink('amqp://localhost');
    });

    it('should connect to rabbitmq', async () => {
        await sink.connect();
        expect(amqp.connect).toHaveBeenCalledWith('amqp://localhost');
        expect(mockConnection.createChannel).toHaveBeenCalled();
    });

    it('should publish a message', async () => {
        await sink.connect();
        await sink.publishAsync('myQueue', { text: 'hello' });
        
        expect(mockChannel.assertQueue).toHaveBeenCalledWith('myQueue', { durable: true });
        expect(mockChannel.sendToQueue).toHaveBeenCalledWith('myQueue', Buffer.from(JSON.stringify({ text: 'hello' })));
    });

    it('should throw an error if not connected', async () => {
        await expect(sink.publishAsync('myQueue', { text: 'hello' })).rejects.toThrow('Not connected. Call connect() first.');
    });

    it('should close connection', async () => {
        await sink.connect();
        await sink.close();
        
        expect(mockChannel.close).toHaveBeenCalled();
        expect(mockConnection.close).toHaveBeenCalled();
    });
});
