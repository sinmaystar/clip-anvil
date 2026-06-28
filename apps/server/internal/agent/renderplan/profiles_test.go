package renderplan

import "testing"

func TestProfileByIDReturnsSeedreamSeedanceAndSeedAudioProfiles(t *testing.T) {
	imageProfile, ok := ProfileByID(ProfileSeedream5Image)
	if !ok {
		t.Fatalf("seedream profile missing")
	}
	if imageProfile.OutputType != "image" || !imageProfile.AllowedOperations["text_to_image"] {
		t.Fatalf("image profile = %#v", imageProfile)
	}
	videoProfile, ok := ProfileByID(ProfileSeedance2Video)
	if !ok {
		t.Fatalf("seedance profile missing")
	}
	if videoProfile.OutputType != "video" || !videoProfile.AllowedOperations["image_to_video_first_frame"] {
		t.Fatalf("video profile = %#v", videoProfile)
	}
	audioProfile, ok := ProfileByID(ProfileSeedAudio1)
	if !ok {
		t.Fatalf("seed audio profile missing")
	}
	if audioProfile.OutputType != "audio" || !audioProfile.AllowedOperations["text_to_audio"] || audioProfile.DefaultModelID != "seed-audio-1.0" {
		t.Fatalf("audio profile = %#v", audioProfile)
	}
}
