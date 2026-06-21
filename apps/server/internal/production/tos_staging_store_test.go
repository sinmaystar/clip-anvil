package production

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos/enum"
)

type fakeTOSClient struct {
	putInput    *tos.PutObjectV2Input
	signInput   *tos.PreSignedURLInput
	signedURL   string
	putContents string
}

func (c *fakeTOSClient) PutObjectV2(_ context.Context, input *tos.PutObjectV2Input) (*tos.PutObjectV2Output, error) {
	c.putInput = input
	data, err := io.ReadAll(input.Content)
	if err != nil {
		return nil, err
	}
	c.putContents = string(data)
	return &tos.PutObjectV2Output{}, nil
}

func (c *fakeTOSClient) PreSignedURL(input *tos.PreSignedURLInput) (*tos.PreSignedURLOutput, error) {
	c.signInput = input
	return &tos.PreSignedURLOutput{SignedUrl: c.signedURL}, nil
}

func TestTOSStagingStoreUsesOfficialSDKInputs(t *testing.T) {
	client := &fakeTOSClient{signedURL: "https://signed.example/input.png"}
	store := newTOSStagingStoreForTest(client, "clip-anvil-temp-bucket")
	store.publicBaseURL = "https://clip-anvil-temp-bucket.tos-cn-beijing.volces.com"

	if err := store.PutObject(context.Background(), "provider-inputs/test.png", strings.NewReader("png"), "image/png"); err != nil {
		t.Fatal(err)
	}
	if client.putInput.Bucket != "clip-anvil-temp-bucket" || client.putInput.Key != "provider-inputs/test.png" {
		t.Fatalf("put bucket/key = %q/%q", client.putInput.Bucket, client.putInput.Key)
	}
	if client.putInput.ContentType != "image/png" || client.putContents != "png" {
		t.Fatalf("put content = %q/%q", client.putInput.ContentType, client.putContents)
	}

	url, err := store.PresignedGetURL(context.Background(), "provider-inputs/test.png", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if url != client.signedURL {
		t.Fatalf("signed url = %q", url)
	}
	if client.signInput.Bucket != "clip-anvil-temp-bucket" || client.signInput.Key != "provider-inputs/test.png" {
		t.Fatalf("sign bucket/key = %q/%q", client.signInput.Bucket, client.signInput.Key)
	}
	if client.signInput.HTTPMethod != enum.HttpMethodGet || client.signInput.Expires != 3600 {
		t.Fatalf("sign method/expires = %q/%d", client.signInput.HTTPMethod, client.signInput.Expires)
	}
	if client.signInput.AlternativeEndpoint != store.publicBaseURL {
		t.Fatalf("alternative endpoint = %q", client.signInput.AlternativeEndpoint)
	}
	if client.signInput.IsCustomDomain == nil || !*client.signInput.IsCustomDomain {
		t.Fatalf("expected bucket public base url to be signed as custom domain")
	}
}

func TestNewTOSStagingStoreRequiresPublicBaseURL(t *testing.T) {
	_, err := NewTOSStagingStore(TOSStagingStoreConfig{
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
		Bucket:          "clip-anvil-temp-bucket",
		Endpoint:        "tos-cn-beijing.volces.com",
		Region:          "cn-beijing",
	})
	if err == nil {
		t.Fatal("expected missing public base url to fail")
	}
}
