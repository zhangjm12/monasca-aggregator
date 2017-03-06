// Copyright 2017 Hewlett Packard Enterprise Development LP
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package main

import (
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	log "github.com/Sirupsen/logrus"

	"github.hpe.com/UNCLE/monasca-aggregation/models"
)

const windowSize = time.Minute/6 // 10 seconds
const windowLag = 2*time.Second

var aggregationSpecifications = []models.AggregationSpecification{
	{"Aggregation0", "metric0", "aggregated-metric0"},
	{"Aggregation1", "metric1", "aggregated-metric1"},
	{"Aggregation2", "metric2", "aggregated-metric2"},
}

var timeWindowAggregations = map[int64]map[string]float64{}

func initLogging() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
}

// Return a timer for when the first window should be processed
// TODO: Check this math to account for all boundary conditions and large lag times
func firstTick() *time.Timer {
	now := time.Now().Unix()
	completed := now % int64(windowSize.Seconds()) - int64(windowLag.Seconds())
	remaining := int64(windowSize.Seconds()) - completed
	firstTick := time.NewTimer(time.Duration(remaining * 1e9))
	return firstTick
}

func publishAggregations(outbound chan *kafka.Message, topic *string) {
	// TODO make timestamp assignment the beginning of the aggregation window
	log.Debug(timeWindowAggregations)
	var currentTimeWindow = int64(time.Now().Unix()) / int64(windowSize.Seconds())
	var windowLagCount = int64(windowLag.Seconds() / windowSize.Seconds()) - 1
	var activeTimeWindow = currentTimeWindow + windowLagCount
	var windowAggregations = timeWindowAggregations[activeTimeWindow]
	log.Infof("currentTimeWindow: %d", currentTimeWindow)
	log.Infof("activeTimeWindow: %d", activeTimeWindow)
	log.Info(windowAggregations)

	for name, value := range windowAggregations {
		var metricEnvelope = models.MetricEnvelope{
			models.Metric{
				name,
				map[string]string{},
				float64(int64(time.Now().Unix()) * 1000),
				value,
				map[string]string{}},
			map[string]string{},
			int64(time.Now().Unix() * 1000)}

		value, _ := json.Marshal(metricEnvelope)

		outbound <- &kafka.Message{TopicPartition: kafka.TopicPartition{Topic: topic, Partition: kafka.PartitionAny}, Value: []byte(value)}
	}

	// TODO: Advance the Kafka offsets
	// TODO: Delete windowAggregations for the current window Id that was just published
	delete(timeWindowAggregations, activeTimeWindow)
}

// TODO: Read in kafka configuration parameters from yaml file
// TODO: Read in aggregation period and aggregation specifications from yaml file
// TODO: Publish aggregated metrics to Kafka
// TODO: Manually update Kafka offsets such that if a crash occurs, processing re-starts at the correct offset
// TODO: Potentially, restrict metrics to a previous, current and next time windowed aggregation.
// TODO: Add support for grouping on dimensions
// TODO: Add Prometheus Client library and report metrics
// TODO: Create Helm Charts
// TODO: Add support for different source and destination Kafka topics.
// TODO: Add support for consuming/publishing intermediary aggregations. For example, publish a (sum, count) to use in an avg aggregation
// TODO: Guarantee at least once publishing of aggregated metrics
// TODO: Handle start/stop, fail/restart
// TODO: Allow start/end consumer offsets to be specified as parameters.
// TODO: Allow start/end aggregation period to be specified.
func main() {
	initLogging()

	if len(os.Args) < 4 {
		log.Errorf("Usage: %s <broker> <group> <topics..>", os.Args[0])
	}

	broker := os.Args[1]
	group := os.Args[2]
	topics := os.Args[3:]

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)

	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":               broker,
		"group.id":                        group,
		"session.timeout.ms":              6000,
		"go.events.channel.enable":        true,
		"go.application.rebalance.enable": true,
		"default.topic.config":            kafka.ConfigMap{"auto.offset.reset": "earliest"}})

	if err != nil {
		log.Fatalf("Failed to create consumer: %s", err)
	}

	log.Infof("Started monasca-aggregation %v", c)

	err = c.SubscribeTopics(topics, nil)

	if err != nil {
		log.Fatalf("Failed to subscribe to topics %c", err)
	}


	p, err := kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": broker})

	if err != nil {
		log.Fatalf("Failed to create producer: %s", err)
	}

	log.Infof("Created Producer %v", p)

	// Set up producer events handling
	go func() {
	outer:
		for e := range p.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				m := ev
				if m.TopicPartition.Error != nil {
					log.Errorf("Delivery failed: %v\n", m.TopicPartition.Error)
				} else {
					log.Infof("Delivered message to topic %s [%d] at offset %v\n",
						*m.TopicPartition.Topic, m.TopicPartition.Partition, m.TopicPartition.Offset)
				}
				break outer

			default:
				log.Debugf("Ignored event: %s\n", ev)
			}
		}
	}()


	firstTick := firstTick()
	var ticker *time.Ticker = new(time.Ticker)

	run := true
	processed_msg_count := 0

	for run == true {
		select {
		case sig := <-sigchan:
			log.Infof("Caught signal %v: terminating", sig)
			run = false

		case ev := <-c.Events():
			switch e := ev.(type) {
			case kafka.AssignedPartitions:
				log.Infof("%% %v", e)
				c.Assign(e.Partitions)
			case kafka.RevokedPartitions:
				log.Infof("%% %v", e)
				c.Unassign()
			case *kafka.Message:
				metricEnvelope := models.MetricEnvelope{}
				err = json.Unmarshal([]byte(e.Value), &metricEnvelope)
				if err != nil {
					log.Warnf("%% Invalid metric envelope on %s:%s", e.TopicPartition, string(e.Value))
					continue
				}
				var metric = metricEnvelope.Metric
				var eventTimeWindow = int64(metric.Timestamp) / (1000 * int64(windowSize.Seconds()))

				for _, aggregationSpecification := range aggregationSpecifications {
					if metric.Name == aggregationSpecification.FilteredMetricName {
						var windowAggregations = timeWindowAggregations[eventTimeWindow]
						if windowAggregations == nil {
							timeWindowAggregations[eventTimeWindow] = make(map[string]float64)
							windowAggregations = timeWindowAggregations[eventTimeWindow]
						}
						windowAggregations[aggregationSpecification.AggregatedMetricName] += metric.Value
					}
				}
				log.Debug(metricEnvelope)
				processed_msg_count += 1
			case kafka.PartitionEOF:
				//log.Infof("%% Reached %v", e)
			case kafka.Error:
				log.Errorf("%% Error: %v", e)
				run = false
			}

		case <- firstTick.C:
			ticker = time.NewTicker(windowSize)
			log.Infof("Processed %d messages", processed_msg_count)
			processed_msg_count = 0
			publishAggregations(p.ProduceChannel(), &topics[0])

		case <- ticker.C:
			log.Infof("Processed %d messages", processed_msg_count)
			processed_msg_count = 0
			publishAggregations(p.ProduceChannel(), &topics[0])
		}
	}

	log.Info("Stopped monasca-aggregation")
	c.Close()
	p.Close()
}
