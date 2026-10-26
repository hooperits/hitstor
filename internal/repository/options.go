package repository

// RepoOptions configures repository runtime behavior.
type RepoOptions struct {
	MinFreeReserveBytes uint64
}

// SetOptions applies runtime options to an opened repository.
func (r *Repository) SetOptions(opts RepoOptions) {
	if opts.MinFreeReserveBytes > 0 {
		r.minFreeReserve = opts.MinFreeReserveBytes
	}
}

func (r *Repository) minFreeRequired() uint64 {
	if r.minFreeReserve > 0 {
		return r.minFreeReserve
	}
	return MinFreeReserveBytes
}
