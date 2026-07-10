#pragma once

#include <string>
#include <vector>
#include <librdkafka/rdkafkacpp.h>
#include <stdexcept>
#include <memory>

namespace helix {
namespace runtime {

class KafkaAsyncSink {
private:
    std::unique_ptr<RdKafka::Producer> producer_;

public:
    KafkaAsyncSink(const std::string& brokers) {
        std::string errstr;
        std::unique_ptr<RdKafka::Conf> conf(RdKafka::Conf::create(RdKafka::Conf::CONF_GLOBAL));
        
        if (conf->set("bootstrap.servers", brokers, errstr) != RdKafka::Conf::CONF_OK) {
            throw std::runtime_error("Failed to set bootstrap.servers: " + errstr);
        }

        producer_.reset(RdKafka::Producer::create(conf.get(), errstr));
        if (!producer_) {
            throw std::runtime_error("Failed to create Kafka producer: " + errstr);
        }
    }

    ~KafkaAsyncSink() {
        if (producer_) {
            producer_->flush(5000);
        }
    }

    void publish_async(const std::string& topic_name, const std::string& key, const std::vector<uint8_t>& payload) {
        RdKafka::ErrorCode err = producer_->produce(
            topic_name,
            RdKafka::Topic::PARTITION_UA,
            RdKafka::Producer::RK_MSG_COPY,
            const_cast<uint8_t*>(payload.data()), payload.size(),
            key.c_str(), key.size(),
            0,
            nullptr, nullptr
        );

        if (err != RdKafka::ERR_NO_ERROR) {
            throw std::runtime_error("Failed to produce message: " + RdKafka::err2str(err));
        }

        producer_->poll(0);
    }
};

} // namespace runtime
} // namespace helix
