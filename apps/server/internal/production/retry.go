package production

import "github.com/jackc/pgx/v5/pgtype"

type RunOptions struct {
	MaxAttempts int
	ParentJobID pgtype.UUID
	Attempt     int
}

func maxAttemptsForRun(options RunOptions, capability Capability) int32 {
	requested := options.MaxAttempts
	if requested <= 0 {
		requested = 1
	}
	if capability.Limits.MaxAttempts > 0 && requested > capability.Limits.MaxAttempts {
		requested = capability.Limits.MaxAttempts
	}
	if requested < 1 {
		requested = 1
	}
	return int32(requested)
}
