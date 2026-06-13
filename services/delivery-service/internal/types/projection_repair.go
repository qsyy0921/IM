package types

type ProjectionCheckpointRepairStats struct {
	Requested int
	Audited   int
	Mutated   int
	Skipped   int
}

const (
	ProjectionCheckpointRepairModeAudit            = "audit"
	ProjectionCheckpointRepairModeRewindNextOffset = "rewind-next-offset"
	ProjectionCheckpointRepairModeRewindFailure    = "rewind-unresolved-failure"
)

type ProjectionCheckpointRepairOptions struct {
	ConsumerGroup string
	Topic         string
	PartitionID   int32
	TargetOffset  int64
	FailureOffset int64
	Mode          string
	Operator      string
	Reason        string
	DryRun        bool
}
