/**
 * Copyright 2016 Confluent Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// kafka client.
// This package implements high-level Apache Kafka producer and consumers
// using bindings on-top of the C librdkafka library.
package kafka

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

/*
#include <librdkafka/rdkafka.h>
*/
import "C"

var testconf struct {
	Brokers string
	Topic   string
}

// testconf_read reads the test suite config file testconf.json which must
// contain at least Brokers and Topic string properties.
// Returns true if the testconf was found and usable, false if no such file, or panics
// if the file format is wrong.
func testconf_read() bool {
	cf, err := os.Open("testconf.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%% testconf.json not found - ignoring test\n")
		return false
	}

	jp := json.NewDecoder(cf)
	err = jp.Decode(&testconf)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse testconf: %s", err))
	}

	cf.Close()

	if testconf.Brokers == "" || testconf.Topic == "" {
		panic("Missing Brokers and Topic in testconf.json")
	}

	return true
}

// ratepdisp tracks and prints message & byte rates
type ratedisp struct {
	name       string
	start      time.Time
	last_print time.Time
	cnt        int64
	size       int64
}

// ratedisp_start sets up a new rate displayer
func ratedisp_start(name string) (pf ratedisp) {
	now := time.Now()
	return ratedisp{name: name, start: now, last_print: now}
}

// reset start time and counters
func (rd *ratedisp) reset() {
	rd.start = time.Now()
	rd.cnt = 0
	rd.size = 0
}

// print the current (accumulated) rate
func (rd *ratedisp) print(pfx string) {
	elapsed := time.Since(rd.start).Seconds()

	fmt.Printf("%s: %s%d messages in %fs (%.0f msgs/s), %d bytes (%.3fMb/s)\n",
		rd.name, pfx, rd.cnt, elapsed, float64(rd.cnt)/elapsed,
		rd.size, (float64(rd.size)/elapsed)/(1024*1024))
}

// tick adds cnt of total size size to the rate displayer and also prints
// running stats every 1s.
func (rd *ratedisp) tick(cnt, size int64) {
	rd.cnt += cnt
	rd.size += size

	if time.Since(rd.last_print).Seconds() >= 1.0 {
		rd.print("")
		rd.last_print = time.Now()
	}
}

// Return the number of messages available in all partitions of a topic.
// WARNING: This uses watermark offsets so it will be incorrect for compacted topics.
func get_message_count_in_topic(topic string) (int, error) {

	// Create consumer
	c, err := NewConsumer(&ConfigMap{"bootstrap.servers": testconf.Brokers})
	if err != nil {
		return 0, err
	}

	// get metadata for the topic to find out number of partitions

	metadata, err := GetMetadata(c, &topic, false, 5*1000)
	if err != nil {
		return 0, err
	}

	t, ok := metadata.Topics[topic]
	if !ok {
		return 0, NewKafkaError(C.RD_KAFKA_RESP_ERR__UNKNOWN_TOPIC)
	}

	cnt := 0
	for _, p := range t.Partitions {
		low, high, err := QueryWatermarkOffsets(c, topic, p.Id, 5*1000)
		if err != nil {
			continue
		}
		cnt += int(high - low)
	}

	return cnt, nil
}