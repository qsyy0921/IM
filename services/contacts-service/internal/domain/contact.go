package domain

import "github.com/qsyy0921/IM/services/contacts-service/internal/types"

func NormalizePageSize(pageSize int) int {
	if pageSize <= 0 {
		return 50
	}
	if pageSize > 100 {
		return 100
	}
	return pageSize
}

func IsTerminalRequestStatus(status types.ContactRequestStatus) bool {
	return status == types.ContactRequestStatusAccepted ||
		status == types.ContactRequestStatusDeclined ||
		status == types.ContactRequestStatusCanceled ||
		status == types.ContactRequestStatusExpired
}
