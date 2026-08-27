package vault

import "fmt"

type CapacityPolicy struct {
	RuntimeReserveBytes int64
}

func (p CapacityPolicy) AdmitBulkWrite(freeBytes, plannedBytes int64) error {
	if freeBytes < 0 || plannedBytes < 0 {
		return fmt.Errorf("invalid capacity values")
	}
	remaining := freeBytes - plannedBytes
	if remaining < p.RuntimeReserveBytes {
		return fmt.Errorf("bulk write would breach runtime reserve: remaining=%d reserve=%d", remaining, p.RuntimeReserveBytes)
	}
	return nil
}
