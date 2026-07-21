package clean

// ServicingObservedFreeBytes sums the post-mutation free-space observations
// across servicing operations and reports whether any positive observation was
// present. A nil observation (not measured) and a measured zero both contribute
// nothing and do not set present: presentation treats measured-zero as no
// observation (it stays recorded in Result/History but is not shown). The
// returned total is an approximate external disk reading, never a reclaimable
// estimate, and callers must never fold it into affected, Recycle Bin, or
// permanent deletion byte totals.
func ServicingObservedFreeBytes(operations []ServicingOperation) (total int64, present bool) {
	for _, op := range operations {
		if op.ObservedFreeBytes != nil && *op.ObservedFreeBytes > 0 {
			total += *op.ObservedFreeBytes
			present = true
		}
	}
	return total, present
}
