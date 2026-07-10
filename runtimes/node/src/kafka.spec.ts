import { KafkaAsyncSink } from './kafka';
import { Kafka, Producer } from 'kafkajs';

jest.mock('kafkajs');

describe('KafkaAsyncSink', () => {
    let sink: KafkaAsyncSink;
    let mockProducer: jest.Mocked<Producer>;
    let mockKafka: jest.Mocked<Kafka>;

    beforeEach(() => {
        jest.clearAllMocks();
        
        mockProducer = {
            connect: jest.fn().mockResolvedValue(undefined),
            send: jest.fn().mockResolvedValue([{}]),
            disconnect: jest.fn().mockResolvedValue(undefined),
        } as unknown as jest.Mocked<Producer>;

        mockKafka = {
            producer: jest.fn().mockReturnValue(mockProducer),
        } as unknown as jest.Mocked<Kafka>;

        (Kafka as unknown as jest.Mock).mockImplementation(() => mockKafka);

        sink = new KafkaAsyncSink('test-client', ['localhost:9092']);
    });

    it('should connect to kafka', async () => {
        await sink.connect();
        expect(mockKafka.producer).toHaveBeenCalled();
        expect(mockProducer.connect).toHaveBeenCalled();
    });

    it('should publish a message', async () => {
        await sink.connect();
        await sink.publishAsync('myTopic', { text: 'hello' });
        
        expect(mockProducer.send).toHaveBeenCalledWith({
            topic: 'myTopic',
            messages: [{ value: JSON.stringify({ text: 'hello' }) }],
        });
    });

    it('should throw an error if not connected', async () => {
        await expect(sink.publishAsync('myTopic', { text: 'hello' })).rejects.toThrow('Not connected. Call connect() first.');
    });

    it('should close connection', async () => {
        await sink.connect();
        await sink.close();
        
        expect(mockProducer.disconnect).toHaveBeenCalled();
    });
});
