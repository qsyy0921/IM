# Kafka Consumer Rebalance Smoke Summary

- Run: kafka-consumer-rebalance-smoke-20260616-clean
- Commit: 79392c64a7573dd28c53f0f5eaf3ee3c62c34c5b
- Git dirty: False
- Scope: local Kafka consumer group rebalance observation; not a production rebalance SLO proof
- Result: passed
- Topic: im.delivery.events
- Consumer group: nexusim-push-rebalance-smoke-20260616170125
- Before stop: state=Stable, members=2, assigned_partitions=3
- After stop: state=Stable, members=1, assigned_partitions=3

## Interpretation

This validates a local push-gateway delivery-consumer group rebalance observation: two consumers join the same group, one process is stopped, and Kafka reassigns the delivery topic partitions to the remaining consumer. It is not a sustained rebalance storm, capacity, or production SLO proof.
