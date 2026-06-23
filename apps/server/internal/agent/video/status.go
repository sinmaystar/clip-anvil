package video

const (
	EventSubmitted = "shot_video_submitted"
	EventSucceeded = "shot_video_succeeded"
	EventFailed    = "shot_video_failed"

	ShotStatusVideoRunning = "video_running"
	ShotStatusVideoReady   = "video_ready"
	ShotStatusFailed       = "failed"
)

func ShotStatusForEvent(event string) (string, bool) {
	switch event {
	case EventSubmitted:
		return ShotStatusVideoRunning, true
	case EventSucceeded:
		return ShotStatusVideoReady, true
	case EventFailed:
		return ShotStatusFailed, true
	default:
		return "", false
	}
}
