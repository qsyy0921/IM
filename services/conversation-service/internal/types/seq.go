package types

import "time"

type SeqBlock struct {
	StartSeq  int64
	EndSeq    int64
	Epoch     int64
	LeaseID   string
	ExpiresAt time.Time
}
