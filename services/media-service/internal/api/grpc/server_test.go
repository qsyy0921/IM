package grpc

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/qsyy0921/IM/services/media-service/internal/types"
)

func TestAssetToProtoDoesNotExposeObjectKey(t *testing.T) {
	asset := types.MediaAsset{
		TenantID:        "tenant-1",
		AssetID:         "asset-1",
		OwnerUserID:     "user-1",
		ConversationID:  "conv-1",
		MediaKind:       types.MediaKindImage,
		ContentType:     "image/png",
		FileName:        "image.png",
		SizeBytes:       64,
		SHA256:          strings.Repeat("a", 64),
		ObjectKey:       "tenant-1/conv-1/internal-object-key",
		Status:          types.AssetStatusReady,
		ScanStatus:      types.ProcessingStatusPassed,
		ThumbnailStatus: types.ProcessingStatusSkipped,
		TranscodeStatus: types.ProcessingStatusSkipped,
		CreatedAt:       time.Unix(1, 0),
		ReadyAt:         time.Unix(2, 0),
	}
	payload, err := protojson.Marshal(assetToProto(asset))
	if err != nil {
		t.Fatalf("marshal asset proto: %v", err)
	}
	if strings.Contains(string(payload), asset.ObjectKey) || strings.Contains(string(payload), "internal-object-key") {
		t.Fatalf("media asset proto leaked object key: %s", payload)
	}
}
