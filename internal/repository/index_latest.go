package repository

type indexLatestBackend interface {
	ReadIndexLatest() (ID, bool, error)
	WriteIndexLatest(ID) error
}

// ReadLatestIndexID returns the consolidated index pointer when available.
func ReadLatestIndexID(be interface{}) (ID, bool, error) {
	lb, ok := be.(indexLatestBackend)
	if !ok {
		return ID{}, false, nil
	}
	return lb.ReadIndexLatest()
}

// SetLatestIndexID records the consolidated index pointer on local backends.
func SetLatestIndexID(be interface{}, id ID) error {
	lb, ok := be.(indexLatestBackend)
	if !ok {
		return nil
	}
	return lb.WriteIndexLatest(id)
}
