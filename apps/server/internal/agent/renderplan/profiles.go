package renderplan

type ModelPromptProfile struct {
	ID                string
	DefaultProvider   string
	DefaultModelID    string
	OutputType        string
	AllowedOperations map[string]bool
	DefaultParams     Params
	MaxPromptChars    int
}

func ProfileByID(id string) (ModelPromptProfile, bool) {
	switch id {
	case ProfileSeedream5Image:
		return ModelPromptProfile{
			ID:              ProfileSeedream5Image,
			DefaultProvider: "volcengine",
			DefaultModelID:  "doubao-seedream-5-0-260128",
			OutputType:      "image",
			AllowedOperations: map[string]bool{
				"text_to_image":        true,
				"image_to_image":       true,
				"multi_image_to_image": true,
			},
			DefaultParams:  Params{Ratio: "9:16", Resolution: "2K", MaxImages: 1},
			MaxPromptChars: 2400,
		}, true
	case ProfileSeedance2Video:
		return ModelPromptProfile{
			ID:              ProfileSeedance2Video,
			DefaultProvider: "volcengine",
			DefaultModelID:  "doubao-seedance-2-0-pro-260428",
			OutputType:      "video",
			AllowedOperations: map[string]bool{
				"text_to_video":                   true,
				"image_to_video_first_frame":      true,
				"image_to_video_first_last_frame": true,
				"multi_modal_reference_video":     true,
				"video_edit":                      true,
				"video_extend":                    true,
				"video_bridge":                    true,
			},
			DefaultParams:  Params{Ratio: "9:16", DurationSec: 5, Resolution: "1080p"},
			MaxPromptChars: 5000,
		}, true
	case ProfileSeedAudio1:
		return ModelPromptProfile{
			ID:              ProfileSeedAudio1,
			DefaultProvider: "volcengine",
			DefaultModelID:  "seed-audio-1.0",
			OutputType:      "audio",
			AllowedOperations: map[string]bool{
				"text_to_audio": true,
			},
			DefaultParams:  Params{Format: "mp3", SampleRate: 48000, Watermark: false},
			MaxPromptChars: 2048,
		}, true
	default:
		return ModelPromptProfile{}, false
	}
}
