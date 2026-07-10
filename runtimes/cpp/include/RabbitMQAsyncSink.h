#pragma once

#include <string>
#include <vector>
#include <memory>
#include <SimpleAmqpClient/SimpleAmqpClient.h>

namespace helix {
namespace runtime {

class RabbitMQAsyncSink {
private:
    std::shared_ptr<AmqpClient::Channel> channel_;
    std::string exchange_;

public:
    RabbitMQAsyncSink(const std::string& amqp_uri, const std::string& exchange) : exchange_(exchange) {
        channel_ = AmqpClient::Channel::CreateFromUri(amqp_uri);
        channel_->DeclareExchange(exchange_, AmqpClient::Channel::EXCHANGE_TYPE_TOPIC, false, true, false);
    }

    void publish_async(const std::string& routing_key, const std::vector<uint8_t>& payload) {
        AmqpClient::BasicMessage::ptr_t msg = AmqpClient::BasicMessage::Create();
        msg->Body(std::string(reinterpret_cast<const char*>(payload.data()), payload.size()));
        msg->DeliveryMode(AmqpClient::BasicMessage::dm_persistent);
        msg->ContentType("application/octet-stream");

        channel_->BasicPublish(exchange_, routing_key, msg);
    }
};

} // namespace runtime
} // namespace helix
