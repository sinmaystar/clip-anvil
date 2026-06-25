package renderplan

import "testing"

func TestProfileByIDReturnsSeedreamAndSeedanceProfiles(t *testing.T) {
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
}
