package types

type DeliveryMessage struct {
	Topic     string
	Partition int
	Offset    int64
	Value     []byte
}
