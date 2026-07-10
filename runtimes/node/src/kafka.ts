import { Kafka, Producer } from 'kafkajs';

export class KafkaAsyncSink {
  private kafka: Kafka;
  private producer: Producer | null = null;

  constructor(clientId: string, brokers: string[]) {
    this.kafka = new Kafka({
      clientId,
      brokers,
    });
  }

  async connect(): Promise<void> {
    if (!this.producer) {
      this.producer = this.kafka.producer();
      await this.producer.connect();
    }
  }

  async close(): Promise<void> {
    if (this.producer) {
      await this.producer.disconnect();
      this.producer = null;
    }
  }

  async publishAsync(topic: string, message: any): Promise<void> {
    if (!this.producer) {
      throw new Error('Not connected. Call connect() first.');
    }
    await this.producer.send({
      topic,
      messages: [{ value: JSON.stringify(message) }],
    });
  }
}
