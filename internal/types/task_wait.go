package types

import "time"

// Media kinds used to pick a per-media async task deadline.
const (
	MediaImage      = "image"
	MediaVideo      = "video"
	MediaMusic      = "music"
	MediaMidjourney = "midjourney"
)

// Built-in async task polling deadlines, used when the media's
// defaults.<media>.timeout is not configured. Generator backends queue work
// differently per media type, so the defaults are deliberately not uniform.
const (
	DefaultImageTaskWait = 10 * time.Minute
	DefaultVideoTaskWait = 30 * time.Minute
	DefaultMusicTaskWait = 15 * time.Minute
	FallbackTaskWait     = 10 * time.Minute
)

// DefaultTaskWait returns the built-in async task polling deadline for a media
// type. Unknown media types fall back to FallbackTaskWait.
func DefaultTaskWait(media string) time.Duration {
	switch media {
	case MediaImage:
		return DefaultImageTaskWait
	case MediaVideo:
		return DefaultVideoTaskWait
	case MediaMusic:
		return DefaultMusicTaskWait
	default:
		return FallbackTaskWait
	}
}
