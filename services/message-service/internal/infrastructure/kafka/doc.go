// Package kafka implements Kafka producers used by trigger jobs.
//
// Business transactions must write outbox rows first; this package must not be
// called directly from SendMessage to bypass the outbox.
package kafka
