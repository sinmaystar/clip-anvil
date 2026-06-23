package preview

const (
	EventDispatched = "preview_dispatched"
	EventSubmitted  = "preview_submitted"
	EventSucceeded  = "preview_succeeded"
	EventFailed     = "preview_failed"

	ShotStatusPreviewRunning = "preview_running"
	ShotStatusPreviewReady   = "preview_ready"
	ShotStatusFailed         = "failed"
)

func ShotStatusForEvent(event string) (string, bool) {
	switch event {
	case EventDispatched, EventSubmitted:
		return ShotStatusPreviewRunning, true
	case EventSucceeded:
		return ShotStatusPreviewReady, true
	case EventFailed:
		return ShotStatusFailed, true
	default:
		return "", false
	}
}
