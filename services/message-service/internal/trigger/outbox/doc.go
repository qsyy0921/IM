// Package outbox contains background workers that publish message_outbox rows
// to Kafka.
//
// It is a trigger layer package. It calls app/infrastructure boundaries and
// must not write message facts outside the transactional outbox model.
package outbox
